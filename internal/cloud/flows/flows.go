package flows

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"syscall"
	"time"

	cloudutils "github.com/coordimap/agent/internal/cloud/utils"
	"github.com/coordimap/agent/pkg/utils"

	"github.com/coordimap/agent/pkg/domain/agent"
	kube_model "github.com/coordimap/agent/pkg/domain/kubernetes"

	"github.com/rs/zerolog/log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func NewFlowsCrawler(dataSource *agent.DataSource, outChannel chan *agent.CloudCrawlData) (Crawler, error) {
	crawler := &flowsCrawler{
		outputChannel: outChannel,
		dataSource:    dataSource,
		kubeClientset: nil,
		podCache:      NewPodCache(),
		interfaceName: "all",
		mappings:      nil,
	}

	for _, dsConfig := range dataSource.Config.ValuePairs {
		switch dsConfig.Key {

		case FLOWS_CONFIG_INTERFACE_NAME:
			crawler.interfaceName = dsConfig.Value

		case FLOWS_CONFIG_EXTERNAL_MAPPINGS:
			mappings, err := cloudutils.NewMappings(dsConfig.Value)
			if err != nil {
				return nil, err
			}
			crawler.mappings = mappings

		case FLOWS_CONFIG_DEPLOYED_AT:
			// deployedAt can only be "kubernetes" or "server" for now
			allowedValues := []string{"kubernetes", "server"}

			if slices.Contains(allowedValues, dsConfig.Value) {
				crawler.deployedAt = dsConfig.Value
			} else {
				log.Warn().Msgf("Invalid value for deployedAt: %s. Allowed values are: %v. Using default 'server'", dsConfig.Value, allowedValues)
				crawler.deployedAt = "server"
			}

		case FLOWS_CONFIG_CRAWL_INTERVAL:
			const DEFAULT_CRAWL_TIME = 30 * time.Second
			amountStr := string(dsConfig.Value[:len(dsConfig.Value)-1])
			durationStr := string(dsConfig.Value[len(dsConfig.Value)-1])

			amount, errConv := strconv.ParseInt(amountStr, 10, 32)
			if errConv != nil {
				return crawler, errConv
			}

			switch durationStr {
			case "s":
				crawler.crawlInterval = time.Duration(amount) * time.Second

			case "m":
				crawler.crawlInterval = time.Duration(amount) * time.Minute

			default:
				crawler.crawlInterval = DEFAULT_CRAWL_TIME
			}
		}
	}

	if crawler.deployedAt == "kubernetes" {
		if crawler.mappings == nil {
			return nil, fmt.Errorf("external_mappings must be set when deployedAt is 'kubernetes'")
		}

		log.Info().Msg("Flows crawler deployedAt is set to 'kubernetes', initializing kube clientset")
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}

		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}

		crawler.kubeClientset = clientset
	}

	return crawler, nil
}

func (crawler *flowsCrawler) Crawl() {
	ctx, cancel := context.WithCancel(context.Background())
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		cancel()
	}()

	var interfaceNames []string
	if crawler.interfaceName == "all" {
		ifaces, err := net.Interfaces()
		if err != nil {
			log.Error().Msgf("Failed to get network interfaces: %s", err.Error())
			return
		}
		for _, iface := range ifaces {
			if iface.Flags&net.FlagLoopback == 0 {
				interfaceNames = append(interfaceNames, iface.Name)
			}
		}
	} else if crawler.interfaceName != "" {
		interfaceNames = append(interfaceNames, crawler.interfaceName)
	} else {
		// Default behavior: find the first non-loopback interface
		ifaces, err := net.Interfaces()
		if err != nil {
			log.Error().Msgf("Failed to get network interfaces: %s", err.Error())
			return
		}
		for _, iface := range ifaces {
			if iface.Flags&net.FlagLoopback == 0 {
				interfaceNames = append(interfaceNames, iface.Name)
				log.Info().Msgf("No interface specified, using first non-loopback interface: %s", iface.Name)
				break
			}
		}
		if len(interfaceNames) == 0 {
			log.Error().Msg("No non-loopback interfaces found")
			return
		}
	}

	var probes []*Probe
	for _, ifaceName := range interfaceNames {
		probe, err := AttachProbe(ifaceName)
		if err != nil {
			log.Error().Msgf("Failed to attach eBPF probe to %s: %s", ifaceName, err.Error())
			continue
		}
		probes = append(probes, probe)
		log.Info().Msgf("Attached eBPF probe to %s", ifaceName)
	}

	if len(probes) == 0 {
		log.Error().Msg("Failed to attach any eBPF probes")
		return
	}

	for _, probe := range probes {
		defer probe.Detach()
	}

	table := NewConnectionTable()

	mergedSamples := make(chan []byte)
	for _, p := range probes {
		go func(probe *Probe) {
			for sample := range probe.Samples {
				mergedSamples <- sample
			}
		}(p)
	}

	for {
		select {
		case <-ctx.Done():
			os.Exit(0)
		case event := <-mergedSamples:
			packet, err := UnmarshalPacket(event)
			if err != nil {
				log.Error().Msgf("Failed to unmarshal packet: %s", err.Error())
				continue
			}

			duration, matched := table.Match(packet)
			if !matched {
				continue
			}

			if crawler.deployedAt == "kubernetes" {
				log.Debug().Msgf("Captured flow: %s:%d -> %s:%d (proto %d) duration %s", packet.SrcIP.String(), packet.SrcPort, packet.DstIP.String(), packet.DstPort, packet.Protocol, duration.String())

				srcPod, errSrcPod := getPodInfo(crawler.kubeClientset, crawler.podCache, packet.SrcIP.String())
				dstPod, errDstPod := getPodInfo(crawler.kubeClientset, crawler.podCache, packet.DstIP.String())
				if errSrcPod != nil || errDstPod != nil {
					continue
				}

				crawler.createAndSendElements(srcPod, dstPod)
			} else {
				log.Info().Msgf("Captured server flow: %s:%d -> %s:%d (proto %d) duration %s", packet.SrcIP.String(), packet.SrcPort, packet.DstIP.String(), packet.DstPort, packet.Protocol, duration.String())
			}
		}
	}
}

func (crawler *flowsCrawler) createAndSendElements(srcPod, dstPod PodInfo) {
	crawledElements := []*agent.Element{}
	crawlTime := time.Now().UTC()
	clusterUID, errGetClusterUIDFromMapping := crawler.mappings.GetValue("*")
	if errGetClusterUIDFromMapping != nil {
		log.Err(errGetClusterUIDFromMapping).Msg("No kubernetes cluster uid")
		return
	}

	srcInternalID := cloudutils.CreateKubeInternalName(clusterUID, srcPod.Namespace, kube_model.TypePod, srcPod.Name)
	dstInternalID := cloudutils.CreateKubeInternalName(clusterUID, dstPod.Namespace, kube_model.TypePod, dstPod.Name)

	srcElement, errSrc := utils.CreateElement(srcPod, srcPod.Name, srcInternalID, kube_model.TypePod, agent.StatusNoStatus, "", crawlTime)
	if errSrc != nil {
		log.Warn().Msgf("Error creating source element: %s", errSrc.Error())
		return
	}

	dstElement, errDst := utils.CreateElement(dstPod, dstPod.Name, dstInternalID, kube_model.TypePod, agent.StatusNoStatus, "", crawlTime)
	if errDst != nil {
		log.Warn().Msgf("Error creating destination element: %s", errDst.Error())
		return
	}

	relation, errRel := utils.CreateRelationship(srcInternalID, dstInternalID, agent.RelationshipType, agent.EBPFFlowTypeRelation, crawlTime)
	if errRel != nil {
		log.Warn().Msgf("Error creating relationship: %s", errRel.Error())
		return
	}

	crawledElements = append(crawledElements, srcElement, dstElement, relation)

	crawledData := agent.CrawledData{
		Data: crawledElements,
	}

	crawler.outputChannel <- &agent.CloudCrawlData{
		Timestamp:       crawlTime,
		DataSource:      *crawler.dataSource,
		CrawledData:     crawledData,
		CrawlInternalID: crawler.dataSource.DataSourceID,
	}
}

func (crawler *flowsCrawler) crawl() (*agent.CloudCrawlData, error) {
	// This function will be called by the ticker, but the main logic is now in Crawl()
	// We can leave this empty or add some periodic tasks if needed.
	return nil, nil
}

func UnmarshalPacket(data []byte) (Packet, error) {
	if len(data) != 48 {
		return Packet{}, fmt.Errorf("slice is not 48 bytes")
	}
	srcIP, ok := netip.AddrFromSlice(data[0:16])
	if !ok {
		panic("invalid source IP")
	}
	dstIP, ok := netip.AddrFromSlice(data[16:32])
	if !ok {
		panic("invalid destination IP")
	}

	return Packet{
		SrcIP:    srcIP,
		DstIP:    dstIP,
		SrcPort:  binary.BigEndian.Uint16(data[32:34]),
		DstPort:  binary.BigEndian.Uint16(data[34:36]),
		Syn:      data[36] == 1,
		Ack:      data[37] == 1,
		Protocol: data[38],
		// 1-byte hole
		Timestamp: Timestamp(binary.LittleEndian.Uint64(data[40:48])),
	}, nil
}

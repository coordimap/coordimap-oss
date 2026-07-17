package flows

import (
	"encoding/binary"
	"hash/fnv"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/coordimap/agent/internal/cloud/utils"

	"github.com/coordimap/agent/pkg/domain/agent"
	"k8s.io/client-go/kubernetes"
)

const (
	FLOWS_CONFIG_CRAWL_INTERVAL    = "crawl_interval"
	FLOWS_CONFIG_DEPLOYED_AT       = "deployedAt"
	FLOWS_CONFIG_INTERFACE_NAME    = "interface_name"
	FLOWS_CONFIG_EXTERNAL_MAPPINGS = "external_mappings"
)

// Crawler is the interface for all the crawlers

type Crawler interface {
	Crawl()
}

type flowsCrawler struct {
	outputChannel chan *agent.CloudCrawlData
	dataSource    *agent.DataSource
	kubeClientset *kubernetes.Clientset
	podCache      *PodCache
	crawlInterval time.Duration
	deployedAt    string
	interfaceName string
	mappings      utils.Mappings
}

type ConnectionData struct {
	SrcIP   net.IP
	DstIP   net.IP
	SrcPort uint16
	DstPort uint16
	Proto   uint8
}

type PodInfo struct {
	Name      string
	Namespace string
	Workload  string
	IP        string
}

type PodCache struct {
	mu    sync.Mutex
	cache map[string]PodInfo
}

func NewPodCache() *PodCache {
	return &PodCache{
		cache: make(map[string]PodInfo),
	}
}

func (c *PodCache) Get(ip string) (PodInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	info, found := c.cache[ip]
	return info, found
}

func (c *PodCache) Set(ip string, info PodInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[ip] = info
}

type Hash uint64

type Timestamp uint64

type Packet struct {
	SrcIP     netip.Addr
	DstIP     netip.Addr
	SrcPort   uint16
	DstPort   uint16
	Syn       bool
	Ack       bool
	Protocol  uint8
	Timestamp Timestamp
}

func (p *Packet) Hash() Hash {
	f := func(v []byte) uint64 {
		h := fnv.New64a()
		h.Write(v)
		return h.Sum64()
	}

	src := binary.BigEndian.AppendUint16(p.SrcIP.AsSlice(), p.SrcPort)
	dst := binary.BigEndian.AppendUint16(p.DstIP.AsSlice(), p.DstPort)

	return Hash(f(src) + f(dst))
}

type ConnectionTable struct {
	table map[Hash]Timestamp
}

func NewConnectionTable() *ConnectionTable {
	return &ConnectionTable{
		table: make(map[Hash]Timestamp),
	}
}

func (c *ConnectionTable) Match(p Packet) (time.Duration, bool) {
	hash := p.Hash()

	timestamp, ok := c.table[hash]
	if ok && p.Ack {
		d := time.Duration(p.Timestamp-timestamp) * time.Nanosecond
		return d, true
	}
	if p.Syn {
		c.table[hash] = p.Timestamp
	}

	return 0, false
}

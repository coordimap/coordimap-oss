package awsflowlogs

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/coordimap/agent/pkg/domain/awsflowlogs"
)

var privateIPBlocks []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // RFC3927 link-local
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique local addr
	} {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Errorf("parse error on %q: %v", cidr, err))
		}
		privateIPBlocks = append(privateIPBlocks, block)
	}
}

func isPrivateIP(ipToCheck string) bool {
	ip := net.ParseIP(ipToCheck)
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

func connectToAWS(region string) (*session.Session, error) {
	config := aws.Config{Region: aws.String(region)}
	sess, errSession := session.NewSession(&config)
	if errSession != nil {
		return nil, fmt.Errorf("problems with connection to AWS because %w", errSession)
	}
	return sess, nil
}

var logFormatFieldIndexes = map[string]map[string]int8{}

func getColumnIndexFromLogFormat(logFormat, columnName string) (int, error) {
	columnName = "${" + columnName + "}"

	logFormatFields, logFormatExists := logFormatFieldIndexes[logFormat]
	if !logFormatExists {
		logFormatSlice := strings.Split(logFormat, " ")
		logFormatFieldIndexes[logFormat] = map[string]int8{}

		foundIndex := -1

		for index, logFormatFieldName := range logFormatSlice {
			logFormatFieldIndexes[logFormat][logFormatFieldName] = int8(index)

			if logFormatFieldName == columnName {
				foundIndex = index
			}
		}

		if foundIndex != -1 {
			return foundIndex, nil
		}
	}

	logFormatFieldIndex, logFormatFieldExists := logFormatFields[columnName]
	if logFormatFieldExists {
		return int(logFormatFieldIndex), nil
	}

	return -1, fmt.Errorf("could not find %s in the log format %s", columnName, logFormat)
}

func getRowValue(row []string, logFormat, columnName string) string {
	colIndex, errColIndex := getColumnIndexFromLogFormat(logFormat, columnName)
	if errColIndex != nil {
		return ""
	}

	return row[colIndex]
}

func isInternalFlow(flow awsflowlogs.AWSFlowLog) bool {
	return isPrivateIP(flow.SrcAddr) && isPrivateIP(flow.DstAddr)
}

func loadState(fileName string) (floLogsState, error) {
	state := floLogsState{LastFileProcessed: ""}
	content, err := os.ReadFile(fileName)
	if err != nil {
		return state, fmt.Errorf("could not open the state file %s because %w", fileName, err)
	}

	err = json.Unmarshal(content, &state)
	if err != nil {
		return state, fmt.Errorf("could not unmarshal the state because %w", err)
	}

	return state, nil
}

func writeState(fileName, lastFileProcessed string) error {
	state := floLogsState{LastFileProcessed: lastFileProcessed}
	content, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("could not marshal state for %s because %w", lastFileProcessed, err)
	}

	err = os.WriteFile(fileName, content, 0644)
	if err != nil {
		return fmt.Errorf("could not write flowLog state file because %w", err)
	}

	return nil
}

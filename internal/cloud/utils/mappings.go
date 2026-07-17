package utils

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

func findMappingValue(configuredMappings map[string]string, mappingToSearchFor string) (string, error) {
	val, ok := configuredMappings[mappingToSearchFor]

	if ok {
		return val, nil
	}

	for key, value := range configuredMappings {
		newKey := strings.Replace(key, "*", ".*", 1)
		regex, errRegex := regexp.Compile(newKey)
		if errRegex != nil {
			return "", fmt.Errorf("could not create regex because %w", errRegex)
		}

		if regex.MatchString(mappingToSearchFor) {
			return value, nil
		}
	}

	return "", errors.New("mapping not found")
}

func GetMappingValue(configuredMappings map[string]string, mappingToSearchFor string) (string, error) {
	return findMappingValue(configuredMappings, mappingToSearchFor)
}

func GetMappingInternalName(configuredMappings map[string]string, mappingToSearchFor string) (string, error) {
	val, err := findMappingValue(configuredMappings, mappingToSearchFor)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%s", val, mappingToSearchFor), nil
}

func GetMappingDataSourceID(configuredMappings map[string]string, mappingToSearchFor string) (string, error) {
	return findMappingValue(configuredMappings, mappingToSearchFor)
}

/**
* SplitConfiguredMappings
* configuredMappings is the string that is taken from the config YAML, it is of the form <mapping_key>@<mapping_value>.
* Depending on the integration, mapping_value can be a data_source_id, a cluster_uid, or another scoped identifier.
 */
func SplitConfiguredMappings(configuredMappings string) (map[string]string, error) {
	mappings := map[string]string{}

	splitString := strings.Split(configuredMappings, " ")

	for _, mapping := range splitString {
		splitMapping := strings.Split(mapping, "@")

		if len(splitMapping) != 2 {
			continue
		}

		mappings[splitMapping[0]] = splitMapping[1]
	}

	return mappings, nil
}

// Mappings defines the interface for managing mappings.
type Mappings interface {
	GetInternalName(mappingToSearchFor string) (string, error)
	GetValue(mappingToSearchFor string) (string, error)
	GetDataSourceID(mappingToSearchFor string) (string, error)
	AddMapping(dataSourceID string, internalName string) error
	AddConfiguredMapping(configuredMappings string) error
}

// mappings holds the configured mappings
type mappings struct {
	mappings map[string]string
}

// NewMappings creates a new Mappings object from a raw configuration string
func NewMappings(configuredMappings string) (Mappings, error) {
	m := &mappings{mappings: make(map[string]string)}
	if err := m.AddConfiguredMapping(configuredMappings); err != nil {
		return nil, err
	}
	return m, nil
}

// GetInternalName returns the internal name for a given mapping
func (m *mappings) GetInternalName(mappingToSearchFor string) (string, error) {
	val, err := m.GetValue(mappingToSearchFor)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%s", val, mappingToSearchFor), nil
}

func (m *mappings) GetValue(mappingToSearchFor string) (string, error) {
	return findMappingValue(m.mappings, mappingToSearchFor)
}

// GetDataSourceID returns the data source ID for a given mapping
func (m *mappings) GetDataSourceID(mappingToSearchFor string) (string, error) {
	return m.GetValue(mappingToSearchFor)
}

// AddMapping adds a new mapping if it doesn't already exist.
func (m *mappings) AddMapping(dataSourceID string, internalName string) error {
	if _, ok := m.mappings[dataSourceID]; ok {
		return fmt.Errorf("mapping for key '%s' already exists", dataSourceID)
	}
	m.mappings[dataSourceID] = internalName
	return nil
}

// AddConfiguredMapping adds new mappings from a raw configuration string.
func (m *mappings) AddConfiguredMapping(configuredMappings string) error {
	splitString := strings.Split(configuredMappings, " ")

	for _, mapping := range splitString {
		splitMapping := strings.Split(mapping, "@")

		if len(splitMapping) != 2 {
			continue
		}

		dataSourceID := splitMapping[0]
		internalName := splitMapping[1]

		if err := m.AddMapping(dataSourceID, internalName); err != nil {
			return err
		}
	}

	return nil
}

package utils

import (
	"testing"
)

func TestMappings(t *testing.T) {
	t.Run("NewMappings", func(t *testing.T) {
		m, err := NewMappings("key1@val1 key2@val2")
		if err != nil {
			t.Fatalf("NewMappings() error = %v", err)
		}

		val, err := m.GetDataSourceID("key1")
		if err != nil || val != "val1" {
			t.Error("mapping for key1 not loaded correctly")
		}

		val, err = m.GetDataSourceID("key2")
		if err != nil || val != "val2" {
			t.Error("mapping for key2 not loaded correctly")
		}

		_, err = NewMappings("key1@val1 key1@val2")
		if err == nil {
			t.Error("Expected error for duplicate key, got nil")
		}
	})

	t.Run("GetInternalName", func(t *testing.T) {
		m, _ := NewMappings("key1@val1")
		got, err := m.GetInternalName("key1")
		if err != nil {
			t.Errorf("GetInternalName() error = %v", err)
		}
		if got != "val1-key1" {
			t.Errorf("GetInternalName() = %v, want %v", got, "val1-key1")
		}

		_, err = m.GetInternalName("key2")
		if err == nil {
			t.Error("Expected error for non-existing key, got nil")
		}
	})

	t.Run("GetDataSourceID", func(t *testing.T) {
		m, _ := NewMappings("key1@val1 key*@val2")
		got, err := m.GetDataSourceID("key1")
		if err != nil {
			t.Errorf("GetDataSourceID() error = %v", err)
		}
		if got != "val1" {
			t.Errorf("GetDataSourceID() = %v, want %v", got, "val1")
		}

		got, err = m.GetDataSourceID("key_anything")
		if err != nil {
			t.Errorf("GetDataSourceID() error = %v", err)
		}
		if got != "val2" {
			t.Errorf("GetDataSourceID() = %v, want %v", got, "val2")
		}

		_, err = m.GetDataSourceID("nonExistingKey")
		if err == nil {
			t.Error("Expected error for non-existing key, got nil")
		}
	})

	t.Run("GetValue", func(t *testing.T) {
		m, _ := NewMappings("cluster-a@uid-a cluster-*@uid-wildcard")

		got, err := m.GetValue("cluster-a")
		if err != nil {
			t.Errorf("GetValue() error = %v", err)
		}
		if got != "uid-a" {
			t.Errorf("GetValue() = %v, want %v", got, "uid-a")
		}

		got, err = m.GetValue("cluster-prod")
		if err != nil {
			t.Errorf("GetValue() wildcard error = %v", err)
		}
		if got != "uid-wildcard" {
			t.Errorf("GetValue() wildcard = %v, want %v", got, "uid-wildcard")
		}
	})

	t.Run("AddMapping", func(t *testing.T) {
		m, _ := NewMappings("")
		err := m.AddMapping("key1", "val1")
		if err != nil {
			t.Errorf("AddMapping() error = %v", err)
		}

		val, err := m.GetDataSourceID("key1")
		if err != nil || val != "val1" {
			t.Error("Mapping not added correctly")
		}

		err = m.AddMapping("key1", "val1")
		if err == nil {
			t.Error("Expected error for existing key, got nil")
		}
	})

	t.Run("AddConfiguredMapping", func(t *testing.T) {
		m, _ := NewMappings("initial@val0")
		err := m.AddConfiguredMapping("key1@val1 key2@val2")
		if err != nil {
			t.Errorf("AddConfiguredMapping() error = %v", err)
		}

		val, err := m.GetDataSourceID("initial")
		if err != nil || val != "val0" {
			t.Error("Initial mapping not preserved")
		}

		val, err = m.GetDataSourceID("key1")
		if err != nil || val != "val1" {
			t.Error("Mapping key1 not added correctly")
		}

		val, err = m.GetDataSourceID("key2")
		if err != nil || val != "val2" {
			t.Error("Mapping key2 not added correctly")
		}

		err = m.AddConfiguredMapping("key1@new_val")
		if err == nil {
			t.Error("Expected error for duplicate key, got nil")
		}
	})
}

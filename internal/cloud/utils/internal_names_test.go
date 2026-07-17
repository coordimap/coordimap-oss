package utils

import (
	"testing"

	kube_model "github.com/coordimap/agent/pkg/domain/kubernetes"
)

func TestCreateKubeInternalName(t *testing.T) {
	got := CreateKubeInternalName("cluster-uid-123", "default", "pod", "api-7c9f")
	want := "cluster-uid-123-default-pod-api-7c9f"

	if got != want {
		t.Fatalf("CreateKubeInternalName() = %q, want %q", got, want)
	}
}

func TestCreateSQLInternalNameKubeUsesClusterUIDScope(t *testing.T) {
	got, err := CreateSQLInternalName("kube:default:orders-db:cluster-uid-123")
	if err != nil {
		t.Fatalf("CreateSQLInternalName() unexpected error: %v", err)
	}

	want := CreateKubeInternalName("cluster-uid-123", "default", kube_model.TypeNamespace, "orders-db")
	if got != want {
		t.Fatalf("CreateSQLInternalName() = %q, want %q", got, want)
	}
}

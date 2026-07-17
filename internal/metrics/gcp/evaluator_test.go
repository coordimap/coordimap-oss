package gcp

import (
	"testing"

	"google.golang.org/api/monitoring/v3"
)

func TestGetFirstPointValue(t *testing.T) {
	doubleValue := 0.8
	intValue := int64(4)
	stringValue := "0.75"

	tests := []struct {
		name      string
		series    *monitoring.TimeSeries
		wantValue float64
		wantFound bool
	}{
		{
			name: "double value",
			series: &monitoring.TimeSeries{Points: []*monitoring.Point{{
				Value: &monitoring.TypedValue{DoubleValue: &doubleValue},
			}}},
			wantValue: 0.8,
			wantFound: true,
		},
		{
			name: "int64 value",
			series: &monitoring.TimeSeries{Points: []*monitoring.Point{{
				Value: &monitoring.TypedValue{Int64Value: &intValue},
			}}},
			wantValue: 4,
			wantFound: true,
		},
		{
			name: "string value",
			series: &monitoring.TimeSeries{Points: []*monitoring.Point{{
				Value: &monitoring.TypedValue{StringValue: &stringValue},
			}}},
			wantValue: 0.75,
			wantFound: true,
		},
		{
			name:      "missing points",
			series:    &monitoring.TimeSeries{},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := getFirstPointValue(tt.series)
			if found != tt.wantFound {
				t.Fatalf("getFirstPointValue() found = %v, want %v", found, tt.wantFound)
			}

			if got != tt.wantValue {
				t.Fatalf("getFirstPointValue() value = %v, want %v", got, tt.wantValue)
			}
		})
	}
}

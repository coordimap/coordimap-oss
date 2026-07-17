package utils

import (
	"bytes"
	"encoding/gob"
	"os"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func TestLoadValueFromEnvConfig(t *testing.T) {
	type args struct {
		value string
		env   string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "test1",
			args:    args{value: "${TEST_ENV1}", env: "TEST_ENV1"},
			want:    "123",
			wantErr: false,
		},
		{
			name:    "test2",
			args:    args{value: "${TEST_ENV2}", env: "TEST_ENV1"},
			want:    "",
			wantErr: true,
		},
		{
			name:    "test3",
			args:    args{value: "${TEST_ENV2", env: "TEST_ENV1"},
			want:    "${TEST_ENV2",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(tt.args.env, tt.want)
			got, err := LoadValueFromEnvConfig(tt.args.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadValueFromEnvConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("LoadValueFromEnvConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_encodeAndHashAWSStruct(t *testing.T) {
	if os.Getenv("RUN_AWS_INTEGRATION_TESTS") == "" {
		t.Skip("set RUN_AWS_INTEGRATION_TESTS=1 to run AWS integration tests")
	}

	type args struct {
		elem interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		want1   string
		wantErr bool
	}{
		{
			name: "test1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			session, _ := session.NewSession(
				&aws.Config{
					Region: aws.String("eu-central-1"),
				},
			)

			accountID := "359635641082"

			svc := ec2.New(session)
			input := &ec2.DescribeInstancesInput{
				Filters: []*ec2.Filter{
					{
						Name:   aws.String("owner-id"),
						Values: []*string{&accountID},
					},
				},
			}

			result, err := svc.DescribeInstances(input)
			if err != nil {
				t.Errorf("encodeAndHashAWSStruct() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			for _, reservation := range result.Reservations {
				for _, elem := range reservation.Instances {
					got, _, err := encodeAndHashAWSStruct(elem)
					if err != nil {
						t.Errorf("encodeAndHashAWSStruct() error = %v, wantErr %v", err, tt.wantErr)
						return
					}

					var instance ec2.Instance
					buffer := bytes.NewBuffer(got)
					decoder := gob.NewDecoder(buffer)

					decoder.Decode(&instance)

					if (err != nil) != tt.wantErr {
						t.Errorf("encodeAndHashAWSStruct() error = %v, wantErr %v", err, tt.wantErr)
						return
					}
					if !reflect.DeepEqual(elem, instance) {
						t.Errorf("encodeAndHashAWSStruct() got = %v, want %v", elem, instance)
					}
				}
			}

		})
	}
}

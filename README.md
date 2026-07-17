# coordimap-local

`coordimap-local` crawls configured infrastructure sources, persists the resulting graph locally, and exposes it through an MCP server over stdio.

## Getting Started

To get started with `coordimap-local`, install Go 1.26 or later.

### Dependencies

`coordimap-local` has the following dependencies:

- [cloud.google.com/go/compute/metadata](http://cloud.google.com/go/compute/metadata)
- [dev.azure.com/bloopi/bloopi/\_git/shared_models.git](http://dev.azure.com/bloopi/bloopi/_git/shared_models.git)
- [github.com/aws/aws-sdk-go](http://github.com/aws/aws-sdk-go)
- [github.com/gertd/go-pluralize](http://github.com/gertd/go-pluralize)
- [github.com/lib/pq](http://github.com/lib/pq)
- [modernc.org/sqlite](https://modernc.org/sqlite)
- [github.com/prometheus/client_golang](http://github.com/prometheus/client_golang)
- [github.com/prometheus/common](http://github.com/prometheus/common)
- [github.com/rs/zerolog](http://github.com/rs/zerolog)
- [github.com/spf13/viper](http://github.com/spf13/viper)
- [go.mongodb.org/mongo-driver](http://go.mongodb.org/mongo-driver)
- [golang.org/x/oauth2](http://golang.org/x/oauth2)
- [google.golang.org/api](http://google.golang.org/api)
- [gopkg.in/alecthomas/kingpin.v2](http://gopkg.in/alecthomas/kingpin.v2)
- [gopkg.in/yaml.v3](http://gopkg.in/yaml.v3)
- [k8s.io/api](http://k8s.io/api)
- [k8s.io/apimachinery](http://k8s.io/apimachinery)
- [k8s.io/client-go](http://k8s.io/client-go)

These dependencies will be automatically downloaded when you build the project.

## Build and Test

Build the local MCP server:

```sh
go build -o coordimap-local ./cmd/coordimap-local
```

Or build its container image:

```sh
docker build -t coordimap-local .
```

The container serves MCP over stdio:

```sh
docker run -i coordimap-local
```

## Configuration

`coordimap-local` reads a YAML configuration file. By default, it looks for `config.yaml` next to the executable; override it with `--config` or `COORDIMAP_CONFIG_PATH`.

The complete configuration shape is shown in `configs/config.yaml.template`. A fully commented version is available in `configs/coordimap-local.example.yaml`.

The configuration file specifies the data sources to be crawled. Here is an example configuration:

```yaml
coordimap:
  data_sources:
    - type: aws
      id: aws-production
      config:
        - name: scope_id
          value: "your-aws-account-id"
        - name: access_key_id
          value: ${AWS_ACCESS_KEY_ID}
        - name: secret_access_key
          value: ${AWS_SECRET_ACCESS_KEY}
        - name: crawl_interval
          value: 30s

    - type: gcp
      id: gcp-production
      config:
        - name: scope_id
          value: "your-gcp-project-number"
        - name: project_id
          value: "your-gcp-project-id"
        - name: credentials_file
          value: /etc/coordimap-local/gcp-service-account.json
        - name: crawl_interval
          value: 30s
```

### Local storage
`coordimap-local` requires `coordimap.database` and persists deduplicated crawl batches locally. It does not send crawled data to an external collector.

```yaml
coordimap:
  database:
    driver: sqlite
    connection_string: file:coordimap.db
  data_sources: []
```

For PostgreSQL, use the same shape with this connection string:

```yaml
coordimap:
  database:
    driver: postgres
    connection_string: postgres://coordimap:password@localhost:5432/coordimap?sslmode=disable
```

### Local MCP server

`coordimap-local` requires `coordimap.database` and serves all MCP protocol traffic over
stdio; do not write secrets or data payloads to stdout outside the JSON-RPC protocol.

Register the executable with an MCP client:

```json
{
  "mcpServers": {
    "coordimap": {
      "command": "coordimap-local",
      "args": ["--config", "/absolute/path/to/config.yaml"]
    }
  }
}
```

Supported data source types are `aws`, `gcp`, `kubernetes`, `postgres`, `mysql`, `mariadb`, `mongodb`, `aws_flow_logs`.

Most crawlers support `crawl_interval` values using seconds or minutes, for example `30s` or `5m`.

## Supported Data Sources

Here are the supported data sources and their sample configurations:

### GCP

```yaml
coordimap:
  data_sources:
    - type: gcp
      id: gcp_id_123
      config:
        - name: scope_id
          value: "your-gcp-project-number"
        - name: in_cloud
          value: "false"
        - name: credentials_file
          value: "/path/to/your/credentials.json"
        - name: project_id
          value: "your-gcp-project-id"
        - name: crawl_interval
          value: 30s
        - name: gcp_flows
          value: "true"
        - name: external_mappings
          value: "europe-west3-your-gke-cluster@your_k8s_cluster_uid"
        - name: include_regions
          value: "your-gcp-region"
```

GCP VPC flow logs are collected by enabling `gcp_flows` on a `gcp` data source.

### AWS

```yaml
- type: aws
  id: awstestid
  config:
    - name: scope_id
      value: "your-aws-account-id"
    - name: policy_config
      value: "true"
    - name: access_key_id
      value: "${AWS_ACCESS_KEY_ID}"
    - name: secret_access_key
      value: "${AWS_SECRET_ACCESS_KEY}"
    - name: crawl_interval
      value: 30s
```

### PostgreSQL

```yaml
- type: postgres
  id: postgres123
  name: "database-name"
  desc: "Description of the database."
  config:
    - name: scope_id
      value: "your-postgres-system-identifier"
    - name: db_name
      value: "your_db_name"
    - name: db_host
      value: "your_db_host"
    - name: db_user
      value: "your_db_user"
    - name: db_pass
      value: "your_db_password"
    - name: ssl_mode
      value: "require" # or disable
    - name: crawl_interval
      value: 30s
    - name: mapping_internal_id
      value: "your-internal-mapping-id"
```

### MariaDB

```yaml
- type: mariadb
  id: "data_source_123"
  config:
    - name: scope_id
      value: "your-mariadb-server-uuid"
    - name: db_name
      value: "your_db_name"
    - name: db_host
      value: "your_db_host"
    - name: db_user
      value: "your_db_user"
    - name: db_pass
      value: "your_db_password"
    - name: crawl_interval
      value: 30s
```

### MySQL

```yaml
- type: mysql
  id: "mysql-primary"
  config:
    - name: scope_id
      value: "your-mysql-server-uuid"
    - name: db_name
      value: "your_db_name"
    - name: db_host
      value: "your_db_host"
    - name: db_user
      value: "your_db_user"
    - name: db_pass
      value: "your_db_password"
    - name: ssl_mode
      value: "disable" # or require
    - name: crawl_interval
      value: 30s
```

### Kubernetes

```yaml
- type: kubernetes
  id: "kube_cluster_id"
  config:
    - name: scope_id
      value: "your_k8s_cluster_uid"
    - name: in_cluster
      value: "false"
    - name: cluster_name
      value: "your_cluster_name"
    - name: cloud_data_source_id
      value: "your_cloud_data_source_id"
    - name: config_file
      value: "/path/to/your/kube/config"
    - name: crawl_interval
      value: 30s
    - name: send_secret_data
      value: "true" # set to "false" to omit Secret data and stringData payloads
    - name: send_configmap_data
      value: "true" # set to "false" to omit ConfigMap data and binaryData payloads
    - name: metrics_prometheus_host
      value: "http://prometheus.monitoring.svc.cluster.local:9090"
    - name: external_mappings
      value: "node-1@aws_data_source_id us-central1-a-node-2@gcp_data_source_id *my-project-id.iam.gserviceaccount.com@123456789012"
```

### Generate Kubernetes Cluster UID

The Kubernetes internal names are scoped by `scope_id` (not by data source id). You should use the cluster UID as the `scope_id`, which you can retrieve from the `kube-system` namespace:

```bash
kubectl get namespace kube-system -o jsonpath='{.metadata.uid}'
```

Use this UID as the `scope_id` in:

- the Kubernetes data source configuration
- mappings that need to reference Kubernetes internal names (for example, GCP flow logs `external_mappings`)

### AWS Flow Logs

```yaml
- type: aws_flow_logs
  name: "flowlog-name"
  desc: "Description of the flow logs."
  config:
    - name: scope_id
      value: "your-aws-account-id"
    - name: log_format
      value: "all"
    - name: log_type
      value: "S3"
    - name: account_id
      value: "your_aws_account_id"
    - name: bucket_name
      value: "your_s3_bucket_name"
    - name: region
      value: "your_aws_region"
    - name: access_key_id
      value: "${AWS_ACCESS_KEY_ID}"
    - name: secret_access_key
      value: "${AWS_SECRET_ACCESS_KEY}"
    - name: crawl_interval
      value: 30s
```

### MongoDB

```yaml
- type: mongodb
  name: "mongo-instance-name"
  desc: "Description of the mongo instance."
  config:
    - name: scope_id
      value: "your-replica-set-id"
    - name: db_name
      value: "*" # or a specific database name
    - name: db_host
      value: "your_mongo_host"
    - name: db_user
      value: "your_mongo_user"
    - name: db_pass
      value: "your_mongo_password"
    - name: crawl_interval
      value: 30s
```

## Identity Matrix

`coordimap-local` uses an internal asset identity model that should be scoped by the upstream system identity, not by the connector `data_source_id`. The `data_source_id` identifies the crawl configuration, while the `scope_id` identifies the real ownership boundary the assets belong to.

| Data source     | Recommended `scope_id`    | Where it comes from                             | Typical asset path                                                                                                     |
| --------------- | ------------------------- | ----------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Kubernetes      | `cluster_uid`             | Kubernetes API cluster identity                 | `namespace/type/name`, `type/name` for cluster-wide assets                                                             |
| GCP             | `project_number`          | GCP project metadata / API                      | `zone/vm_instance/name`, `region/bucket/name`, `region/sql/name`                                                       |
| AWS             | `account_id`              | AWS STS caller identity                         | `region/ec2/instance-id`, `region/rds/db-arn`, `global/s3/bucket-name`                                                 |
| PostgreSQL      | `system_identifier`       | PostgreSQL server or cluster identity           | `database/schema/table`, `database/schema/index`                                                                       |
| MySQL / MariaDB | `server_uuid`             | MySQL or MariaDB server identity                | `database/schema/table`, `database/schema/index`                                                                       |
| MongoDB         | replica set or cluster ID | replica set or cluster identity                 | `database/collection`, `database/collection/index`                                                                     |
| OTel            | reuse upstream `scope_id` | OTel resource attributes from the source system | `coordimap.scope_id`, `k8s.cluster.uid`, `cloud.account.id`, `cloud.project.number`, `db.postgresql.system_identifier` |

## How To Find Your `scope_id`

You must provide a `scope_id` for each data source in your configuration. This keeps internal asset IDs stable across configurations and data source recreation. Use the following guidelines to find the appropriate `scope_id` for your data sources.

### Kubernetes

Use the cluster UID:

```bash
kubectl get namespace kube-system -o jsonpath='{.metadata.uid}'
```

Recommended `scope_id`: `cluster_uid`

Notes:

- Works for both self-hosted and managed Kubernetes clusters.
- This is the preferred scope for Kubernetes internal names.
- When linking Kubernetes service accounts to GCP service accounts for Workload Identity,
  use `external_mappings` entries that match the full GSA email suffix and map it to the
  GCP `scope_id` (project number), for example:

```yaml
- name: external_mappings
  value: "*my-project-id.iam.gserviceaccount.com@123456789012"
```

- In that mapping:
  - `my-project-id` is the GCP project ID used in the GSA email address
  - `123456789012` is the GCP project number, which is the recommended GCP `scope_id`

### GCP

Use the project number:

```bash
gcloud projects describe PROJECT_ID --format='value(projectNumber)'
```

You can also get the project ID if needed:

```bash
gcloud projects describe PROJECT_ID --format='value(projectId)'
```

Recommended `scope_id`: `project_number`

### AWS

Use the AWS account ID:

```bash
aws sts get-caller-identity --query Account --output text
```

Recommended `scope_id`: `account_id`

### PostgreSQL

Use the PostgreSQL system identifier:

```sql
SELECT system_identifier FROM pg_control_system();
```

Recommended `scope_id`: `system_identifier`

Notes:

- This is the preferred scope for self-hosted PostgreSQL.
- It identifies the PostgreSQL server or cluster lineage.
- If `pg_control_system()` is unavailable, the value can also be retrieved with PostgreSQL system tooling such as `pg_controldata`.

### MySQL / MariaDB

Use the server UUID:

```sql
SHOW VARIABLES LIKE 'server_uuid';
```

Recommended `scope_id`: `server_uuid`

Notes:

- This is the preferred scope for self-hosted MySQL and MariaDB instances.
- If `server_uuid` is not available in a specific deployment, use an explicitly configured stable `scope_id`.

### MongoDB

Use a replica set or cluster identity when available.

Useful commands:

```javascript
rs.conf();
rs.status();
```

Recommended `scope_id`: replica set / cluster ID

Notes:

- For replica set deployments, prefer a true replica set or cluster identifier if your deployment exposes one.
- If no immutable ID is available, the replica set name is an acceptable but weaker fallback.
- For standalone MongoDB instances, use an explicitly configured stable `scope_id`.

### OTel

OTel should reuse the upstream `scope_id` instead of inventing a separate identity.

Recommended resource attributes include:

- `coordimap.scope_id`
- `k8s.cluster.uid`
- `cloud.account.id`
- `cloud.project.number`
- `db.postgresql.system_identifier`

Notes:

- OTel should emit the same scope used by the infrastructure crawler.
- This allows the backend to generate matching internal IDs and create relationships reliably.

## Metric Trigger Rules

`coordimap-local` evaluates metric rules and persists metric-trigger elements locally. These are regular elements with type `coordimap.metric_trigger` and include all matching internal IDs in the element payload.

Metric rules are configured inside each datasource block (`data_sources[*].metric_rules`).
Currently metric rules are supported only for datasource types:

- `kubernetes`
- `gcp`

### Supported Providers

- `prometheus` (for Kubernetes data sources)
- `gcp_monitoring` (for GCP data sources)

### Config Format

Metric rules are configured in YAML under `data_sources[*].metric_rules`.

Each rule must set `mode` to either:

- `custom` for user-defined provider queries
- `predefined` for built-in templates

Example:

```yaml
coordimap:
  data_sources:
    - type: kubernetes
      id: kube-prod
      config:
        - name: scope_id
          value: your-cluster-uid
        - name: config_file
          value: /path/to/your/kube/config
        - name: metrics_prometheus_host
          value: http://prometheus.monitoring.svc.cluster.local:9090
      metric_rules:
        - id: k8s-high-5xx
          name: Kubernetes Service High 5xx
          provider: prometheus
          mode: custom
          custom:
            query: sum(rate(istio_requests_total{response_code=~"5.."}[5m])) by (destination_workload_namespace, destination_canonical_service)
          lookback: 5m
          threshold:
            operator: ">"
            value: 1
          target:
            resolver: kubernetes_service
            namespace_label: destination_workload_namespace
            name_label: destination_canonical_service

    - type: gcp
      id: gcp-prod
      config:
        - name: scope_id
          value: "123456789012"
        - name: project_id
          value: your-project-id
      metric_rules:
        - id: cloudsql-high-cpu
          name: CloudSQL High CPU
          provider: gcp_monitoring
          mode: predefined
          predefined:
            name: cloudsql_high_cpu
            params:
              lookback: 5m
              threshold: 0.8
```

Kubernetes metric rules require either `metrics_prometheus_host` or `prometheus_host` in the same Kubernetes data source configuration.

Common fields:

- `id`
- `name`
- `provider`
- `lookback`
- `threshold.operator` and `threshold.value`
- `target.resolver`

Prometheus-specific:

- `custom.query`

GCP Monitoring-specific:

- `custom.filter` or `custom.metric_type`
- optional `alignment_period`, `per_series_aligner`, `cross_series_reducer`, `group_by_fields`

### Predefined Rules

Current predefined templates:

- provider `prometheus`
  - `kubernetes_service_high_5xx`
  - `kubernetes_deployment_high_5xx`
  - `kubernetes_service_high_latency`
  - `kubernetes_pod_high_restart_rate`
  - `kubernetes_pod_crashloop_or_imagepull_error`
  - `kubernetes_pod_not_ready`
  - `kubernetes_deployment_unavailable_replicas`
  - `kubernetes_deployment_availability_gap`
  - `kubernetes_pod_high_cpu_usage`
  - `kubernetes_pod_high_memory_workingset`
  - `kubernetes_pod_cpu_throttling_high`
  - `kubernetes_pod_oom_events`
  - `kubernetes_pod_unschedulable`
  - `kubernetes_pvc_low_free_space`
  - `kubernetes_pvc_free_space_burn_rate`
  - `kubernetes_inode_low_free`
  - `kubernetes_statefulset_pvc_low_free_space`
- provider `gcp_monitoring`
  - `cloudsql_high_cpu`
  - `cloudsql_high_connections`
  - `vm_high_cpu`

Predefined params are template-specific. For example:

- `kubernetes_service_high_5xx`: `window`, `threshold`
- `kubernetes_deployment_high_5xx`: `window`, `threshold`
- `kubernetes_service_high_latency`: `window`, `quantile`, `threshold`
- `kubernetes_pod_high_restart_rate`: `window`, `threshold`
- `kubernetes_pod_crashloop_or_imagepull_error`: `lookback`, `reason_regex`
- `kubernetes_pod_not_ready`: `lookback`
- `kubernetes_deployment_unavailable_replicas`: `threshold`
- `kubernetes_deployment_availability_gap`: `threshold`
- `kubernetes_pod_high_cpu_usage`: `window`, `threshold`
- `kubernetes_pod_high_memory_workingset`: `threshold`
- `kubernetes_pod_cpu_throttling_high`: `window`, `threshold`
- `kubernetes_pod_oom_events`: `window`
- `kubernetes_pod_unschedulable`: `lookback`
- `kubernetes_pvc_low_free_space`: `threshold`, `namespace`, `pvc_regex`
- `kubernetes_pvc_free_space_burn_rate`: `threshold`, `window`, `horizon_seconds`, `namespace`, `pvc_regex`
- `kubernetes_inode_low_free`: `threshold`, `namespace`, `pvc_regex`
- `kubernetes_statefulset_pvc_low_free_space`: `threshold`, `namespace`, `statefulset`, `volume_claim_prefix`
- `cloudsql_high_cpu`: `lookback`, `threshold`
- `cloudsql_high_connections`: `lookback`, `threshold`, `metric_type`, `alignment_period`, `per_series_aligner`
- `vm_high_cpu`: `lookback`, `threshold`, `alignment_period`, `per_series_aligner`

### Target Resolvers

- Kubernetes: `kubernetes_service`, `kubernetes_deployment`, `kubernetes_pod`, `kubernetes_pvc`, `kubernetes_statefulset`
- GCP: `gcp_cloudsql`, `gcp_vm_instance`
- Cross data source: `external_mapping`

For `external_mapping`, if no `external_mappings` entry matches, the target is ignored and nothing is sent for that series.

## Contribute

If you would like to contribute to `coordimap-local`, please fork the repository and submit a pull request. We welcome all contributions!

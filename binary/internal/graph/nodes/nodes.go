package nodes

type NodeType string

const (
	TypePostgres   NodeType = "postgres"
	TypeMongodb    NodeType = "mongodb"
	TypeRedis      NodeType = "redis"
	TypeS3         NodeType = "s3"
	TypeDatabase   NodeType = "database"
	TypeTable      NodeType = "table"
	TypeCollection NodeType = "collection"
	TypeBucket     NodeType = "bucket"
	TypeStorage    NodeType = "storage"
	TypeService    NodeType = "service"
	TypeApi        NodeType = "api"
	TypeGateway    NodeType = "gateway"
	TypeAuth       NodeType = "auth"
	TypeQueue         NodeType = "queue"
	TypeCache         NodeType = "cache"
	TypeMySQL         NodeType = "mysql"
	TypeElasticsearch NodeType = "elasticsearch"
	TypeIndex         NodeType = "index"

	// Kubernetes resource types.
	TypeNamespace   NodeType = "namespace"
	TypeDeployment  NodeType = "deployment"
	TypeStatefulSet NodeType = "statefulset"
	TypeDaemonSet   NodeType = "daemonset"
	TypePod         NodeType = "pod"
	TypeK8sService  NodeType = "k8s_service"
)

type Node struct {
	Id       string         `json:"id"`
	Type     string         `json:"type"`
	Name     string         `json:"name"`
	Parent   string         `json:"parent,omitempty"`
	Metadata map[string]any `json:"metadata"`
	Health   string         `json:"health"`
}

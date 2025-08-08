package v1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// AutoscalingSpec определяет конфигурацию автоматического масштабирования
type AutoscalingSpec struct {
	// HPA конфигурация Horizontal Pod Autoscaler
	HPA *HPASpec `json:"hpa,omitempty"`

	// VPA конфигурация Vertical Pod Autoscaler
	VPA *VPASpec `json:"vpa,omitempty"`
}

// HPASpec определяет конфигурацию Horizontal Pod Autoscaler
type HPASpec struct {
	// Enabled включает/выключает HPA
	Enabled bool `json:"enabled"`

	// MinReplicas минимальное количество реплик
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas максимальное количество реплик
	MaxReplicas int32 `json:"maxReplicas"`

	// TargetCPUUtilizationPercentage целевая утилизация CPU в процентах
	TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`

	// TargetMemoryUtilizationPercentage целевая утилизация памяти в процентах
	TargetMemoryUtilizationPercentage *int32 `json:"targetMemoryUtilizationPercentage,omitempty"`

	// Metrics пользовательские метрики для масштабирования
	Metrics []MetricSpec `json:"metrics,omitempty"`

	// Behavior поведение масштабирования (scale up/down policies)
	Behavior *HPABehavior `json:"behavior,omitempty"`
}

// VPASpec определяет конфигурацию Vertical Pod Autoscaler
type VPASpec struct {
	// Enabled включает/выключает VPA
	Enabled bool `json:"enabled"`

	// UpdateMode режим обновления VPA (Off, Initial, Recreation, Auto)
	UpdateMode VPAUpdateMode `json:"updateMode,omitempty"`

	// ResourcePolicy политика управления ресурсами
	ResourcePolicy *VPAResourcePolicy `json:"resourcePolicy,omitempty"`
}

// MetricSpec определяет пользовательскую метрику для HPA
type MetricSpec struct {
	// Type тип метрики (Resource, Pods, Object, External)
	Type MetricType `json:"type"`

	// Resource метрика ресурса
	Resource *ResourceMetricSpec `json:"resource,omitempty"`

	// Pods метрика подов
	Pods *PodsMetricSpec `json:"pods,omitempty"`

	// Object метрика объекта
	Object *ObjectMetricSpec `json:"object,omitempty"`

	// External внешняя метрика
	External *ExternalMetricSpec `json:"external,omitempty"`
}

// HPABehavior определяет поведение масштабирования
type HPABehavior struct {
	// ScaleUp политика масштабирования вверх
	ScaleUp *HPAScalingRules `json:"scaleUp,omitempty"`

	// ScaleDown политика масштабирования вниз
	ScaleDown *HPAScalingRules `json:"scaleDown,omitempty"`
}

// HPAScalingRules правила масштабирования
type HPAScalingRules struct {
	// StabilizationWindowSeconds окно стабилизации в секундах
	StabilizationWindowSeconds *int32 `json:"stabilizationWindowSeconds,omitempty"`

	// SelectPolicy политика выбора (Max, Min, Disabled)
	SelectPolicy *ScalingPolicySelect `json:"selectPolicy,omitempty"`

	// Policies список политик масштабирования
	Policies []HPAScalingPolicy `json:"policies,omitempty"`
}

// HPAScalingPolicy политика масштабирования
type HPAScalingPolicy struct {
	// Type тип политики (Percent, Pods)
	Type HPAScalingPolicyType `json:"type"`

	// Value значение для политики
	Value int32 `json:"value"`

	// PeriodSeconds период в секундах
	PeriodSeconds int32 `json:"periodSeconds"`
}

// VPAResourcePolicy политика управления ресурсами VPA
type VPAResourcePolicy struct {
	// ContainerPolicies политики для контейнеров
	ContainerPolicies []VPAContainerResourcePolicy `json:"containerPolicies,omitempty"`
}

// VPAContainerResourcePolicy политика ресурсов для контейнера
type VPAContainerResourcePolicy struct {
	// ContainerName имя контейнера
	ContainerName string `json:"containerName,omitempty"`

	// Mode режим управления ресурсами (Auto, Off)
	Mode VPAContainerScalingMode `json:"mode,omitempty"`

	// MinAllowed минимальные разрешенные ресурсы
	MinAllowed ResourceList `json:"minAllowed,omitempty"`

	// MaxAllowed максимальные разрешенные ресурсы
	MaxAllowed ResourceList `json:"maxAllowed,omitempty"`

	// ControlledResources контролируемые ресурсы
	ControlledResources []corev1.ResourceName `json:"controlledResources,omitempty"`

	// ControlledValues контролируемые значения (RequestsAndLimits, RequestsOnly)
	ControlledValues VPAControlledValues `json:"controlledValues,omitempty"`
}

// Enums for autoscaling configuration

// VPAUpdateMode режим обновления VPA
type VPAUpdateMode string

const (
	VPAUpdateModeOff        VPAUpdateMode = "Off"
	VPAUpdateModeInitial    VPAUpdateMode = "Initial"
	VPAUpdateModeRecreation VPAUpdateMode = "Recreation"
	VPAUpdateModeAuto       VPAUpdateMode = "Auto"
)

// VPAContainerScalingMode режим масштабирования контейнера
type VPAContainerScalingMode string

const (
	VPAContainerScalingModeAuto VPAContainerScalingMode = "Auto"
	VPAContainerScalingModeOff  VPAContainerScalingMode = "Off"
)

// VPAControlledValues контролируемые значения VPA
type VPAControlledValues string

const (
	VPAControlledValuesRequestsAndLimits VPAControlledValues = "RequestsAndLimits"
	VPAControlledValuesRequestsOnly      VPAControlledValues = "RequestsOnly"
)

// MetricType тип метрики
type MetricType string

const (
	MetricTypeResource MetricType = "Resource"
	MetricTypePods     MetricType = "Pods"
	MetricTypeObject   MetricType = "Object"
	MetricTypeExternal MetricType = "External"
)

// ScalingPolicySelect политика выбора масштабирования
type ScalingPolicySelect string

const (
	ScalingPolicySelectMax      ScalingPolicySelect = "Max"
	ScalingPolicySelectMin      ScalingPolicySelect = "Min"
	ScalingPolicySelectDisabled ScalingPolicySelect = "Disabled"
)

// HPAScalingPolicyType тип политики масштабирования HPA
type HPAScalingPolicyType string

const (
	HPAScalingPolicyTypePercent HPAScalingPolicyType = "Percent"
	HPAScalingPolicyTypePods    HPAScalingPolicyType = "Pods"
)

// Metric specification types (simplified versions)

// ResourceMetricSpec спецификация метрики ресурса
type ResourceMetricSpec struct {
	// Name имя ресурса
	Name corev1.ResourceName `json:"name"`

	// Target целевое значение
	Target MetricTarget `json:"target"`
}

// PodsMetricSpec спецификация метрики подов
type PodsMetricSpec struct {
	// Metric описание метрики
	Metric MetricIdentifier `json:"metric"`

	// Target целевое значение
	Target MetricTarget `json:"target"`
}

// ObjectMetricSpec спецификация метрики объекта
type ObjectMetricSpec struct {
	// DescribedObject описываемый объект
	DescribedObject CrossVersionObjectReference `json:"describedObject"`

	// Metric описание метрики
	Metric MetricIdentifier `json:"metric"`

	// Target целевое значение
	Target MetricTarget `json:"target"`
}

// ExternalMetricSpec спецификация внешней метрики
type ExternalMetricSpec struct {
	// Metric описание метрики
	Metric MetricIdentifier `json:"metric"`

	// Target целевое значение
	Target MetricTarget `json:"target"`
}

// MetricTarget целевое значение метрики
type MetricTarget struct {
	// Type тип целевого значения (Utilization, Value, AverageValue)
	Type MetricTargetType `json:"type"`

	// Value абсолютное значение
	Value *resource.Quantity `json:"value,omitempty"`

	// AverageValue среднее значение
	AverageValue *resource.Quantity `json:"averageValue,omitempty"`

	// AverageUtilization средняя утилизация в процентах
	AverageUtilization *int32 `json:"averageUtilization,omitempty"`
}

// MetricIdentifier идентификатор метрики
type MetricIdentifier struct {
	// Name имя метрики
	Name string `json:"name"`

	// Selector селектор метрики
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

// CrossVersionObjectReference ссылка на объект
type CrossVersionObjectReference struct {
	// Kind вид объекта
	Kind string `json:"kind"`

	// Name имя объекта
	Name string `json:"name"`

	// APIVersion версия API
	APIVersion string `json:"apiVersion,omitempty"`
}

// MetricTargetType тип целевого значения метрики
type MetricTargetType string

const (
	MetricTargetTypeUtilization  MetricTargetType = "Utilization"
	MetricTargetTypeValue        MetricTargetType = "Value"
	MetricTargetTypeAverageValue MetricTargetType = "AverageValue"
)

// ConfigSource определяет источник конфигурации
type ConfigSource struct {
	// ConfigMap ссылка на ConfigMap
	ConfigMap *ConfigMapSource `json:"configMap,omitempty"`

	// Secret ссылка на Secret
	Secret *SecretSource `json:"secret,omitempty"`

	// MountPath путь монтирования конфигурации
	MountPath string `json:"mountPath"`

	// SubPath подпуть в источнике конфигурации
	SubPath string `json:"subPath,omitempty"`

	// ReadOnly указывает, что конфигурация только для чтения
	ReadOnly bool `json:"readOnly,omitempty"`
}

// ConfigMapSource ссылка на ConfigMap
type ConfigMapSource struct {
	// Name имя ConfigMap
	Name string `json:"name"`

	// Keys список ключей для загрузки (если не указан, загружаются все)
	Keys []string `json:"keys,omitempty"`

	// Optional указывает, что ConfigMap может отсутствовать
	Optional bool `json:"optional,omitempty"`
}

// SecretSource ссылка на Secret
type SecretSource struct {
	// Name имя Secret
	Name string `json:"name"`

	// Keys список ключей для загрузки (если не указан, загружаются все)
	Keys []string `json:"keys,omitempty"`

	// Optional указывает, что Secret может отсутствовать
	Optional bool `json:"optional,omitempty"`
}

// SecretReference ссылка на секрет для конфиденциальных данных
type SecretReference struct {
	// Name имя секрета
	Name string `json:"name"`

	// Key ключ в секрете
	Key string `json:"key"`

	// EnvVar имя переменной окружения
	EnvVar string `json:"envVar"`

	// Optional указывает, что секрет может отсутствовать
	Optional bool `json:"optional,omitempty"`
}

// EnvFromSource источник переменных окружения
type EnvFromSource struct {
	// ConfigMapRef ссылка на ConfigMap
	ConfigMapRef *ConfigMapEnvSource `json:"configMapRef,omitempty"`

	// SecretRef ссылка на Secret
	SecretRef *SecretEnvSource `json:"secretRef,omitempty"`

	// Prefix префикс для переменных окружения
	Prefix string `json:"prefix,omitempty"`
}

// ConfigMapEnvSource источник переменных окружения из ConfigMap
type ConfigMapEnvSource struct {
	// Name имя ConfigMap
	Name string `json:"name"`

	// Optional указывает, что ConfigMap может отсутствовать
	Optional bool `json:"optional,omitempty"`
}

// SecretEnvSource источник переменных окружения из Secret
type SecretEnvSource struct {
	// Name имя Secret
	Name string `json:"name"`

	// Optional указывает, что Secret может отсутствовать
	Optional bool `json:"optional,omitempty"`
}

// VolumeMount описывает монтирование тома
type VolumeMount struct {
	// Name имя тома
	Name string `json:"name"`

	// MountPath путь монтирования в контейнере
	MountPath string `json:"mountPath"`

	// SubPath подпуть в томе
	SubPath string `json:"subPath,omitempty"`

	// ReadOnly указывает, что том только для чтения
	ReadOnly bool `json:"readOnly,omitempty"`

	// MountPropagation определяет, как монтирование распространяется
	MountPropagation *MountPropagationMode `json:"mountPropagation,omitempty"`

	// SubPathExpr расширяет subPath с переменными окружения
	SubPathExpr string `json:"subPathExpr,omitempty"`
}

// Volume описывает том
type Volume struct {
	// Name имя тома
	Name string `json:"name"`

	// MCPVolumeSource источник тома
	MCPVolumeSource MCPVolumeSource `json:",inline"`
}

// MCPVolumeSource источник тома для MCP
type MCPVolumeSource struct {
	// ConfigMap том из ConfigMap
	ConfigMap *ConfigMapVolumeSource `json:"configMap,omitempty"`

	// Secret том из Secret
	Secret *SecretVolumeSource `json:"secret,omitempty"`

	// EmptyDir пустой том
	EmptyDir *EmptyDirVolumeSource `json:"emptyDir,omitempty"`

	// PersistentVolumeClaim том из PVC
	PersistentVolumeClaim *PVCVolumeSource `json:"persistentVolumeClaim,omitempty"`

	// HostPath том из хост-системы
	HostPath *HostPathVolumeSource `json:"hostPath,omitempty"`

	// Projected проецируемый том
	Projected *ProjectedVolumeSource `json:"projected,omitempty"`
}

// ConfigMapVolumeSource том из ConfigMap
type ConfigMapVolumeSource struct {
	// Name имя ConfigMap
	Name string `json:"name"`

	// Items список элементов для проекции
	Items []KeyToPath `json:"items,omitempty"`

	// DefaultMode права доступа по умолчанию
	DefaultMode *int32 `json:"defaultMode,omitempty"`

	// Optional указывает, что ConfigMap может отсутствовать
	Optional bool `json:"optional,omitempty"`
}

// SecretVolumeSource том из Secret
type SecretVolumeSource struct {
	// Name имя Secret
	Name string `json:"name"`

	// Items список элементов для проекции
	Items []KeyToPath `json:"items,omitempty"`

	// DefaultMode права доступа по умолчанию
	DefaultMode *int32 `json:"defaultMode,omitempty"`

	// Optional указывает, что Secret может отсутствовать
	Optional bool `json:"optional,omitempty"`
}

// ProjectedVolumeSource проецируемый том
type ProjectedVolumeSource struct {
	// Sources источники для проекции
	Sources []VolumeProjection `json:"sources"`

	// DefaultMode права доступа по умолчанию
	DefaultMode *int32 `json:"defaultMode,omitempty"`
}

// VolumeProjection проекция тома
type VolumeProjection struct {
	// ConfigMap проекция из ConfigMap
	ConfigMap *ConfigMapProjection `json:"configMap,omitempty"`

	// Secret проекция из Secret
	Secret *SecretProjection `json:"secret,omitempty"`

	// ServiceAccountToken проекция токена сервисного аккаунта
	ServiceAccountToken *ServiceAccountTokenProjection `json:"serviceAccountToken,omitempty"`
}

// ConfigMapProjection проекция ConfigMap
type ConfigMapProjection struct {
	// Name имя ConfigMap
	Name string `json:"name"`

	// Items список элементов для проекции
	Items []KeyToPath `json:"items,omitempty"`

	// Optional указывает, что ConfigMap может отсутствовать
	Optional bool `json:"optional,omitempty"`
}

// SecretProjection проекция Secret
type SecretProjection struct {
	// Name имя Secret
	Name string `json:"name"`

	// Items список элементов для проекции
	Items []KeyToPath `json:"items,omitempty"`

	// Optional указывает, что Secret может отсутствовать
	Optional bool `json:"optional,omitempty"`
}

// ServiceAccountTokenProjection проекция токена сервисного аккаунта
type ServiceAccountTokenProjection struct {
	// Audience аудитория токена
	Audience string `json:"audience,omitempty"`

	// ExpirationSeconds время жизни токена в секундах
	ExpirationSeconds *int64 `json:"expirationSeconds,omitempty"`

	// Path путь к файлу токена
	Path string `json:"path"`
}

// KeyToPath сопоставление ключа к пути
type KeyToPath struct {
	// Key ключ в источнике
	Key string `json:"key"`

	// Path путь к файлу
	Path string `json:"path"`

	// Mode права доступа к файлу
	Mode *int32 `json:"mode,omitempty"`
}

// MountPropagationMode режим распространения монтирования
type MountPropagationMode string

const (
	// MountPropagationNone монтирование не распространяется
	MountPropagationNone MountPropagationMode = "None"
	// MountPropagationHostToContainer монтирование распространяется от хоста к контейнеру
	MountPropagationHostToContainer MountPropagationMode = "HostToContainer"
	// MountPropagationBidirectional двунаправленное распространение монтирования
	MountPropagationBidirectional MountPropagationMode = "Bidirectional"
)

// TenancySpec определяет конфигурацию мультитенантности
type TenancySpec struct {
	// TenantID идентификатор тенанта
	TenantID string `json:"tenantId"`

	// IsolationLevel уровень изоляции тенанта
	IsolationLevel IsolationLevel `json:"isolationLevel,omitempty"`

	// ResourceQuotas квоты ресурсов для тенанта
	ResourceQuotas *TenantResourceQuotas `json:"resourceQuotas,omitempty"`

	// NetworkPolicy политика сетевой изоляции
	NetworkPolicy *TenantNetworkPolicy `json:"networkPolicy,omitempty"`

	// Labels дополнительные метки для тенанта
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations дополнительные аннотации для тенанта
	Annotations map[string]string `json:"annotations,omitempty"`
}

// TenantResourceQuotas определяет квоты ресурсов для тенанта
type TenantResourceQuotas struct {
	// CPU максимальное количество CPU для тенанта
	CPU *resource.Quantity `json:"cpu,omitempty"`

	// Memory максимальное количество памяти для тенанта
	Memory *resource.Quantity `json:"memory,omitempty"`

	// Storage максимальное количество хранилища для тенанта
	Storage *resource.Quantity `json:"storage,omitempty"`

	// Pods максимальное количество подов для тенанта
	Pods *int32 `json:"pods,omitempty"`

	// Services максимальное количество сервисов для тенанта
	Services *int32 `json:"services,omitempty"`

	// PersistentVolumeClaims максимальное количество PVC для тенанта
	PersistentVolumeClaims *int32 `json:"persistentVolumeClaims,omitempty"`

	// ConfigMaps максимальное количество ConfigMaps для тенанта
	ConfigMaps *int32 `json:"configMaps,omitempty"`

	// Secrets максимальное количество Secrets для тенанта
	Secrets *int32 `json:"secrets,omitempty"`
}

// TenantNetworkPolicy определяет политику сетевой изоляции для тенанта
type TenantNetworkPolicy struct {
	// Enabled включает/выключает сетевую изоляцию
	Enabled bool `json:"enabled"`

	// AllowedNamespaces список разрешенных namespace для взаимодействия
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`

	// AllowedPods селектор разрешенных подов для взаимодействия
	AllowedPods *metav1.LabelSelector `json:"allowedPods,omitempty"`

	// IngressRules правила входящего трафика
	IngressRules []NetworkPolicyIngressRule `json:"ingressRules,omitempty"`

	// EgressRules правила исходящего трафика
	EgressRules []NetworkPolicyEgressRule `json:"egressRules,omitempty"`
}

// NetworkPolicyIngressRule правило входящего трафика
type NetworkPolicyIngressRule struct {
	// Ports список портов
	Ports []NetworkPolicyPort `json:"ports,omitempty"`

	// From источники трафика
	From []NetworkPolicyPeer `json:"from,omitempty"`
}

// NetworkPolicyEgressRule правило исходящего трафика
type NetworkPolicyEgressRule struct {
	// Ports список портов
	Ports []NetworkPolicyPort `json:"ports,omitempty"`

	// To назначения трафика
	To []NetworkPolicyPeer `json:"to,omitempty"`
}

// NetworkPolicyPort определяет порт для сетевой политики
type NetworkPolicyPort struct {
	// Protocol протокол (TCP, UDP, SCTP)
	Protocol *corev1.Protocol `json:"protocol,omitempty"`

	// Port номер порта
	Port *intstr.IntOrString `json:"port,omitempty"`

	// EndPort конечный порт для диапазона
	EndPort *int32 `json:"endPort,omitempty"`
}

// NetworkPolicyPeer определяет источник или назначение трафика
type NetworkPolicyPeer struct {
	// PodSelector селектор подов
	PodSelector *metav1.LabelSelector `json:"podSelector,omitempty"`

	// NamespaceSelector селектор namespace
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// IPBlock блок IP-адресов
	IPBlock *IPBlock `json:"ipBlock,omitempty"`
}

// IPBlock определяет блок IP-адресов
type IPBlock struct {
	// CIDR блок IP-адресов в формате CIDR
	CIDR string `json:"cidr"`

	// Except исключения из CIDR блока
	Except []string `json:"except,omitempty"`
}

// IsolationLevel уровень изоляции тенанта
type IsolationLevel string

const (
	// IsolationLevelNone без изоляции
	IsolationLevelNone IsolationLevel = "None"
	// IsolationLevelNamespace изоляция на уровне namespace
	IsolationLevelNamespace IsolationLevel = "Namespace"
	// IsolationLevelNode изоляция на уровне узла
	IsolationLevelNode IsolationLevel = "Node"
	// IsolationLevelStrict строгая изоляция
	IsolationLevelStrict IsolationLevel = "Strict"
)

// MCPServerSpec определяет желаемое состояние MCPServer
type MCPServerSpec struct {
	// Registry содержит информацию о сервере из MCP Registry
	Registry MCPRegistryInfo `json:"registry"`

	// Runtime определяет среду выполнения сервера
	Runtime MCPRuntimeSpec `json:"runtime"`

	// Config содержит конфигурацию сервера
	Config *runtime.RawExtension `json:"config,omitempty"`

	// Replicas определяет количество реплик сервера
	Replicas *int32 `json:"replicas,omitempty"`

	// Resources определяет требования к ресурсам
	Resources ResourceRequirements `json:"resources,omitempty"`

	// Environment содержит переменные окружения
	Environment map[string]string `json:"environment,omitempty"`

	// ServiceAccount для запуска подов
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// Selector для выбора подов (если не указан, генерируется автоматически)
	Selector map[string]string `json:"selector,omitempty"`

	// SecurityContext определяет контекст безопасности для подов
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`

	// ServiceType тип сервиса Kubernetes
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`

	// NodeSelector для размещения подов на определенных узлах
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations для размещения подов на узлах с taint
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity правила размещения подов
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Autoscaling конфигурация автоматического масштабирования
	Autoscaling *AutoscalingSpec `json:"autoscaling,omitempty"`

	// ConfigSources определяет источники конфигурации (ConfigMaps, Secrets)
	ConfigSources []ConfigSource `json:"configSources,omitempty"`

	// SecretRefs ссылки на секреты для конфиденциальных данных
	SecretRefs []SecretReference `json:"secretRefs,omitempty"`

	// EnvFrom загружает переменные окружения из ConfigMaps и Secrets
	EnvFrom []EnvFromSource `json:"envFrom,omitempty"`

	// VolumeMounts монтирование томов с конфигурацией
	VolumeMounts []VolumeMount `json:"volumeMounts,omitempty"`

	// Volumes дополнительные тома для контейнера
	Volumes []Volume `json:"volumes,omitempty"`

	// Tenancy конфигурация мультитенантности
	Tenancy *TenancySpec `json:"tenancy,omitempty"`
}

// MCPRegistryInfo содержит информацию о сервере из реестра
type MCPRegistryInfo struct {
	// Name название сервера в реестре
	Name string `json:"name"`

	// Version версия сервера
	Version string `json:"version,omitempty"`

	// Description описание сервера
	Description string `json:"description,omitempty"`

	// Repository URL репозитория
	Repository string `json:"repository,omitempty"`

	// License лицензия сервера
	License string `json:"license,omitempty"`

	// Author автор сервера
	Author string `json:"author,omitempty"`

	// Keywords ключевые слова
	Keywords []string `json:"keywords,omitempty"`

	// Capabilities возможности сервера
	Capabilities []string `json:"capabilities,omitempty"`
}

// MCPRuntimeSpec определяет среду выполнения
type MCPRuntimeSpec struct {
	// Type тип среды выполнения (docker, node, python)
	Type string `json:"type"`

	// Image Docker образ (для type: docker)
	Image string `json:"image,omitempty"`

	// Command команда запуска
	Command []string `json:"command,omitempty"`

	// Args аргументы команды
	Args []string `json:"args,omitempty"`

	// Env переменные окружения для runtime
	Env map[string]string `json:"env,omitempty"`

	// Port порт для подключения к MCP серверу
	Port int32 `json:"port,omitempty"`
}

// ResourceRequirements определяет требования к ресурсам
type ResourceRequirements struct {
	// Limits максимальные лимиты ресурсов
	Limits ResourceList `json:"limits,omitempty"`

	// Requests запрашиваемые ресурсы
	Requests ResourceList `json:"requests,omitempty"`
}

// ResourceList карта ресурсов
type ResourceList map[string]string

// MCPServerStatus определяет наблюдаемое состояние MCPServer
type MCPServerStatus struct {
	// Phase текущая фаза жизненного цикла сервера
	Phase MCPServerPhase `json:"phase,omitempty"`

	// Message человекочитаемое сообщение о состоянии
	Message string `json:"message,omitempty"`

	// Reason причина текущего состояния
	Reason string `json:"reason,omitempty"`

	// Replicas текущее количество реплик
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas количество готовых реплик
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// AvailableReplicas количество доступных реплик
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// Conditions условия состояния
	Conditions []MCPServerCondition `json:"conditions,omitempty"`

	// ServiceEndpoint endpoint для подключения к серверу
	ServiceEndpoint string `json:"serviceEndpoint,omitempty"`

	// LastUpdateTime время последнего обновления
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
}

// MCPServerPhase представляет фазу жизненного цикла
type MCPServerPhase string

const (
	// MCPServerPhasePending сервер в процессе создания
	MCPServerPhasePending MCPServerPhase = "Pending"

	// MCPServerPhaseRunning сервер запущен и работает
	MCPServerPhaseRunning MCPServerPhase = "Running"

	// MCPServerPhaseFailed сервер в состоянии ошибки
	MCPServerPhaseFailed MCPServerPhase = "Failed"

	// MCPServerPhaseTerminating сервер в процессе удаления
	MCPServerPhaseTerminating MCPServerPhase = "Terminating"
)

// MCPServerCondition описывает условие состояния MCPServer
type MCPServerCondition struct {
	// Type тип условия
	Type MCPServerConditionType `json:"type"`

	// Status статус условия (True, False, Unknown)
	Status metav1.ConditionStatus `json:"status"`

	// LastTransitionTime время последнего изменения условия
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`

	// Reason машиночитаемая причина изменения условия
	Reason string `json:"reason,omitempty"`

	// Message человекочитаемое сообщение
	Message string `json:"message,omitempty"`
}

// MCPServerConditionType тип условия
type MCPServerConditionType string

const (
	// MCPServerConditionReady сервер готов к работе
	MCPServerConditionReady MCPServerConditionType = "Ready"

	// MCPServerConditionAvailable сервер доступен
	MCPServerConditionAvailable MCPServerConditionType = "Available"

	// MCPServerConditionProgressing сервер в процессе развертывания
	MCPServerConditionProgressing MCPServerConditionType = "Progressing"

	// MCPServerConditionRegistryFetched спецификация успешно загружена из реестра
	MCPServerConditionRegistryFetched MCPServerConditionType = "RegistryFetched"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.status.replicas`
//+kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
//+kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.status.serviceEndpoint`

// MCPServer is the Schema for the mcpservers API
type MCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPServerSpec   `json:"spec,omitempty"`
	Status MCPServerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MCPServerList contains a list of MCPServer
type MCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MCPServer{}, &MCPServerList{})
}

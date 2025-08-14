package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
)

var (
	// Trusted image registries
	trustedRegistries = []string{
		"docker.io",
		"ghcr.io",
		"quay.io",
		"gcr.io",
		"registry.k8s.io",
	}

	// Maximum resource limits
	maxCPU    = resource.MustParse("2")
	maxMemory = resource.MustParse("4Gi")
)

// MCPServerValidator validates MCPServer resources
type MCPServerValidator struct {
	client.Client
}

// MCPServerDefaulter sets defaults for MCPServer resources
type MCPServerDefaulter struct {
	client.Client
}

// +kubebuilder:webhook:path=/validate-mcp-allbeone-io-v1-mcpserver,mutating=false,failurePolicy=fail,sideEffects=None,groups=mcp.allbeone.io,resources=mcpservers,verbs=create;update,versions=v1,name=vmcpserver.kb.io,admissionReviewVersions=v1

func (v *MCPServerValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	mcpServer := &mcpv1.MCPServer{}

	if err := json.Unmarshal(req.Object.Raw, mcpServer); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Validate the MCPServer
	if allErrs := v.validateMCPServer(mcpServer); len(allErrs) > 0 {
		return admission.Denied(allErrs.ToAggregate().Error())
	}

	return admission.Allowed("")
}

// +kubebuilder:webhook:path=/mutate-mcp-allbeone-io-v1-mcpserver,mutating=true,failurePolicy=fail,sideEffects=None,groups=mcp.allbeone.io,resources=mcpservers,verbs=create;update,versions=v1,name=mmcpserver.kb.io,admissionReviewVersions=v1

func (d *MCPServerDefaulter) Handle(ctx context.Context, req admission.Request) admission.Response {
	mcpServer := &mcpv1.MCPServer{}

	if err := json.Unmarshal(req.Object.Raw, mcpServer); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Set defaults
	d.setDefaults(mcpServer)

	marshaledMCPServer, err := json.Marshal(mcpServer)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledMCPServer)
}

func (v *MCPServerValidator) validateMCPServer(mcpServer *mcpv1.MCPServer) field.ErrorList {
	var allErrs field.ErrorList

	// Validate image security
	if errs := v.validateImageSecurity(mcpServer); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	// Validate resource limits
	if errs := v.validateResourceLimits(mcpServer); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	// Validate replicas
	if errs := v.validateReplicas(mcpServer); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	// Validate security context
	if errs := v.validateSecurityContext(mcpServer); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	// Validate tenant configuration
	if errs := v.validateTenantConfiguration(mcpServer); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	return allErrs
}

func (v *MCPServerValidator) validateImageSecurity(mcpServer *mcpv1.MCPServer) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// Only require image for docker runtime type
	if mcpServer.Spec.Runtime != nil && mcpServer.Spec.Runtime.Type == "docker" {
		image := mcpServer.Spec.Runtime.Image
		if image == "" {
			allErrs = append(allErrs, field.Required(specPath.Child("runtime", "image"), "image is required for docker runtime"))
			return allErrs
		}

		// Validate image registry for docker runtime
		isTrusted := false
		for _, registry := range trustedRegistries {
			if strings.HasPrefix(image, registry) {
				isTrusted = true
				break
			}
		}

		if !isTrusted {
			allErrs = append(allErrs, field.Invalid(
				specPath.Child("runtime", "image"),
				image,
				fmt.Sprintf("image must be from trusted registry: %v", trustedRegistries),
			))
		}

		// Validate image tag (no latest in production)
		if strings.HasSuffix(image, ":latest") {
			allErrs = append(allErrs, field.Invalid(
				specPath.Child("runtime", "image"),
				image,
				"using 'latest' tag is not recommended for production",
			))
		}
	}

	return allErrs
}

func (v *MCPServerValidator) validateResourceLimits(mcpServer *mcpv1.MCPServer) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	if mcpServer.Spec.Resources.Limits == nil {
		allErrs = append(allErrs, field.Required(specPath.Child("resources", "limits"), "resource limits are required"))
		return allErrs
	}

	limits := mcpServer.Spec.Resources.Limits

	// Validate CPU limits
	if cpuStr, exists := limits["cpu"]; exists {
		if cpu, err := resource.ParseQuantity(cpuStr); err == nil {
			if cpu.Cmp(maxCPU) > 0 {
				allErrs = append(allErrs, field.Invalid(
					specPath.Child("resources", "limits", "cpu"),
					cpuStr,
					fmt.Sprintf("CPU limit cannot exceed %s", maxCPU.String()),
				))
			}
		}
	}

	// Validate Memory limits
	if memoryStr, exists := limits["memory"]; exists {
		if memory, err := resource.ParseQuantity(memoryStr); err == nil {
			if memory.Cmp(maxMemory) > 0 {
				allErrs = append(allErrs, field.Invalid(
					specPath.Child("resources", "limits", "memory"),
					memoryStr,
					fmt.Sprintf("Memory limit cannot exceed %s", maxMemory.String()),
				))
			}
		}
	}

	return allErrs
}

func (v *MCPServerValidator) validateReplicas(mcpServer *mcpv1.MCPServer) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	replicas := mcpServer.Spec.Replicas
	if replicas != nil && *replicas < 0 {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("replicas"),
			*replicas,
			"replicas cannot be negative",
		))
	}

	if replicas != nil && *replicas > 10 {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("replicas"),
			*replicas,
			"replicas cannot exceed 10 for security reasons",
		))
	}

	return allErrs
}

func (v *MCPServerValidator) validateSecurityContext(mcpServer *mcpv1.MCPServer) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// Ensure non-root user
	if mcpServer.Spec.SecurityContext != nil {
		if mcpServer.Spec.SecurityContext.RunAsUser != nil && *mcpServer.Spec.SecurityContext.RunAsUser == 0 {
			allErrs = append(allErrs, field.Invalid(
				specPath.Child("securityContext", "runAsUser"),
				*mcpServer.Spec.SecurityContext.RunAsUser,
				"running as root (UID 0) is not allowed",
			))
		}

		if mcpServer.Spec.SecurityContext.RunAsNonRoot != nil && !*mcpServer.Spec.SecurityContext.RunAsNonRoot {
			allErrs = append(allErrs, field.Invalid(
				specPath.Child("securityContext", "runAsNonRoot"),
				*mcpServer.Spec.SecurityContext.RunAsNonRoot,
				"runAsNonRoot must be true",
			))
		}
	}

	return allErrs
}

func (v *MCPServerValidator) validateTenantConfiguration(mcpServer *mcpv1.MCPServer) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	if mcpServer.Spec.Tenancy != nil {
		tenancyPath := specPath.Child("tenancy")

		// Validate tenant ID format
		if mcpServer.Spec.Tenancy.TenantID == "" {
			allErrs = append(allErrs, field.Required(tenancyPath.Child("tenantId"), "tenant ID is required when tenancy is configured"))
		} else if len(mcpServer.Spec.Tenancy.TenantID) > 63 {
			allErrs = append(allErrs, field.Invalid(
				tenancyPath.Child("tenantId"),
				mcpServer.Spec.Tenancy.TenantID,
				"tenant ID cannot exceed 63 characters",
			))
		}

		// Validate resource quotas
		if mcpServer.Spec.Tenancy.ResourceQuotas != nil {
			quotasPath := tenancyPath.Child("resourceQuotas")

			if mcpServer.Spec.Tenancy.ResourceQuotas.Pods != nil && *mcpServer.Spec.Tenancy.ResourceQuotas.Pods <= 0 {
				allErrs = append(allErrs, field.Invalid(
					quotasPath.Child("pods"),
					*mcpServer.Spec.Tenancy.ResourceQuotas.Pods,
					"pod quota must be positive",
				))
			}

			if mcpServer.Spec.Tenancy.ResourceQuotas.Services != nil && *mcpServer.Spec.Tenancy.ResourceQuotas.Services <= 0 {
				allErrs = append(allErrs, field.Invalid(
					quotasPath.Child("services"),
					*mcpServer.Spec.Tenancy.ResourceQuotas.Services,
					"service quota must be positive",
				))
			}
		}
	}

	return allErrs
}

func (d *MCPServerDefaulter) setDefaults(mcpServer *mcpv1.MCPServer) {
	// Initialize registry if nil
	if mcpServer.Spec.Registry == nil {
		mcpServer.Spec.Registry = &mcpv1.RegistryRef{}
	}

	// Default registry.server to metadata.name if empty
	if mcpServer.Spec.Registry.Server == "" {
		mcpServer.Spec.Registry.Server = mcpServer.Name
	}

	// Default registry.registry to "default-registry" if empty
	if mcpServer.Spec.Registry.Registry == "" {
		mcpServer.Spec.Registry.Registry = "default-registry"
	}

	// Convert old spec.port to spec.ports if spec.ports is empty
	if mcpServer.Spec.Port != nil && len(mcpServer.Spec.Ports) == 0 {
		mcpServer.Spec.Ports = []mcpv1.PortSpec{
			{
				Name:       "http",
				Port:       *mcpServer.Spec.Port,
				TargetPort: *mcpServer.Spec.Port,
				Protocol:   "TCP",
			},
		}
	}

	// Set defaults for each port in spec.ports
	for i := range mcpServer.Spec.Ports {
		port := &mcpServer.Spec.Ports[i]
		// If targetPort is empty, set it to port
		if port.TargetPort == 0 {
			port.TargetPort = port.Port
		}
		// If protocol is empty, set it to TCP
		if port.Protocol == "" {
			port.Protocol = "TCP"
		}
	}

	// Set default transport path "/mcp" when type is specified but path is empty
	if mcpServer.Spec.Transport != nil && mcpServer.Spec.Transport.Type != "" && mcpServer.Spec.Transport.Path == "" {
		mcpServer.Spec.Transport.Path = "/mcp"
	}

	// Set default replicas
	if mcpServer.Spec.Replicas == nil {
		replicas := int32(1)
		mcpServer.Spec.Replicas = &replicas
	}

	// Set default port
	if mcpServer.Spec.Runtime != nil && mcpServer.Spec.Runtime.Port == 0 {
		mcpServer.Spec.Runtime.Port = 8080
	}

	// Set default service type
	if mcpServer.Spec.ServiceType == "" {
		mcpServer.Spec.ServiceType = corev1.ServiceTypeClusterIP
	}

	// Set default security context
	if mcpServer.Spec.SecurityContext == nil {
		runAsNonRoot := true
		runAsUser := int64(1000)
		mcpServer.Spec.SecurityContext = &corev1.SecurityContext{
			RunAsNonRoot: &runAsNonRoot,
			RunAsUser:    &runAsUser,
		}
	}

	// Set default resource requests if not specified
	if mcpServer.Spec.Resources.Requests == nil {
		mcpServer.Spec.Resources.Requests = make(mcpv1.ResourceList)
		mcpServer.Spec.Resources.Requests["cpu"] = "100m"
		mcpServer.Spec.Resources.Requests["memory"] = "128Mi"
	}

	// Set default resource limits if not specified
	if mcpServer.Spec.Resources.Limits == nil {
		mcpServer.Spec.Resources.Limits = make(mcpv1.ResourceList)
		mcpServer.Spec.Resources.Limits["cpu"] = "500m"
		mcpServer.Spec.Resources.Limits["memory"] = "512Mi"
	}

	// Set default tenancy isolation level
	if mcpServer.Spec.Tenancy != nil && mcpServer.Spec.Tenancy.IsolationLevel == "" {
		mcpServer.Spec.Tenancy.IsolationLevel = mcpv1.IsolationLevelNamespace
	}
}

func SetupWebhookWithManager(mgr ctrl.Manager) error {
	validator := &MCPServerValidator{
		Client: mgr.GetClient(),
	}

	defaulter := &MCPServerDefaulter{
		Client: mgr.GetClient(),
	}

	mgr.GetWebhookServer().Register("/validate-mcp-allbeone-io-v1-mcpserver", &webhook.Admission{Handler: validator})
	mgr.GetWebhookServer().Register("/mutate-mcp-allbeone-io-v1-mcpserver", &webhook.Admission{Handler: defaulter})

	return nil
}

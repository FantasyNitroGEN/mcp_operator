package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/registry"
)

// MCPServerReconciler reconciles a MCPServer object
type MCPServerReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	RegistryClient *registry.Client
}

//+kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpservers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpservers/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MCPServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logs := log.FromContext(ctx)

	// Получаем MCPServer ресурс
	var mcpServer mcpv1.MCPServer
	if err := r.Get(ctx, req.NamespacedName, &mcpServer); err != nil {
		if errors.IsNotFound(err) {
			// Ресурс был удален, ничего не делаем
			return ctrl.Result{}, nil
		}
		logs.Error(err, "unable to fetch MCPServer")
		return ctrl.Result{}, err
	}

	// Обновляем статус на Pending если это новый ресурс
	if mcpServer.Status.Phase == "" {
		mcpServer.Status.Phase = mcpv1.MCPServerPhasePending
		mcpServer.Status.Message = "Starting MCP server deployment"
		mcpServer.Status.LastUpdateTime = &metav1.Time{Time: time.Now()}
		if err := r.Status().Update(ctx, &mcpServer); err != nil {
			logs.Error(err, "unable to update MCPServer status")
			return ctrl.Result{}, err
		}
	}

	// Получаем спецификацию из реестра если нужно
	if err := r.enrichFromRegistry(ctx, &mcpServer); err != nil {
		logs.Error(err, "unable to enrich from registry")
		r.updateStatus(ctx, &mcpServer, mcpv1.MCPServerPhaseFailed,
			"Failed to get server spec from registry", err.Error())
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}

	// Создаем или обновляем Deployment
	if err := r.reconcileDeployment(ctx, &mcpServer); err != nil {
		logs.Error(err, "unable to reconcile Deployment")
		r.updateStatus(ctx, &mcpServer, mcpv1.MCPServerPhaseFailed,
			"Failed to create deployment", err.Error())
		return ctrl.Result{}, err
	}

	// Создаем или обновляем Service
	if err := r.reconcileService(ctx, &mcpServer); err != nil {
		logs.Error(err, "unable to reconcile Service")
		r.updateStatus(ctx, &mcpServer, mcpv1.MCPServerPhaseFailed,
			"Failed to create service", err.Error())
		return ctrl.Result{}, err
	}

	// Обновляем статус
	if err := r.updateStatusFromDeployment(ctx, &mcpServer); err != nil {
		logs.Error(err, "unable to update status from deployment")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Minute * 2}, nil
}

// enrichFromRegistry обогащает MCPServer данными из реестра
func (r *MCPServerReconciler) enrichFromRegistry(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	if r.RegistryClient == nil {
		return nil // Регистр не настроен
	}

	// Если данные уже есть, пропускаем
	if mcpServer.Spec.Registry.Version != "" && mcpServer.Spec.Runtime.Image != "" {
		return nil
	}

	spec, err := r.RegistryClient.GetServerSpec(ctx, mcpServer.Spec.Registry.Name)
	if err != nil {
		return fmt.Errorf("failed to get server spec: %w", err)
	}

	// Обогащаем данными из реестра
	mcpServer.Spec.Registry.Version = spec.Version
	mcpServer.Spec.Registry.Description = spec.Description
	mcpServer.Spec.Registry.Repository = spec.Repository
	mcpServer.Spec.Registry.License = spec.License
	mcpServer.Spec.Registry.Author = spec.Author
	mcpServer.Spec.Registry.Keywords = spec.Keywords
	mcpServer.Spec.Registry.Capabilities = spec.Capabilities

	// Обогащаем runtime если не задан
	if mcpServer.Spec.Runtime.Type == "" {
		mcpServer.Spec.Runtime.Type = spec.Runtime.Type
	}
	if mcpServer.Spec.Runtime.Image == "" {
		mcpServer.Spec.Runtime.Image = spec.Runtime.Image
	}
	if len(mcpServer.Spec.Runtime.Command) == 0 {
		mcpServer.Spec.Runtime.Command = spec.Runtime.Command
	}
	if len(mcpServer.Spec.Runtime.Args) == 0 {
		mcpServer.Spec.Runtime.Args = spec.Runtime.Args
	}

	return r.Update(ctx, mcpServer)
}

// reconcileDeployment создает или обновляет Deployment
func (r *MCPServerReconciler) reconcileDeployment(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	deployment := &appsv1.Deployment{}
	deploymentName := mcpServer.Name

	err := r.Get(ctx, types.NamespacedName{
		Name:      deploymentName,
		Namespace: mcpServer.Namespace,
	}, deployment)

	if errors.IsNotFound(err) {
		// Создаем новый Deployment
		deployment = r.createDeploymentForMCPServer(mcpServer)
		if err := controllerutil.SetControllerReference(mcpServer, deployment, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, deployment)
	} else if err != nil {
		return err
	}

	// Обновляем существующий Deployment если нужно
	// Здесь можно добавить логику сравнения и обновления
	return nil
}

// createDeploymentForMCPServer создает Deployment для MCPServer
func (r *MCPServerReconciler) createDeploymentForMCPServer(mcpServer *mcpv1.MCPServer) *appsv1.Deployment {
	replicas := int32(1)
	if mcpServer.Spec.Replicas != nil {
		replicas = *mcpServer.Spec.Replicas
	}

	labels := r.labelsForMCPServer(mcpServer)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "mcp-server",
							Image:   mcpServer.Spec.Runtime.Image,
							Command: mcpServer.Spec.Runtime.Command,
							Args:    mcpServer.Spec.Runtime.Args,
							Ports: []corev1.ContainerPort{
								{
									Name:          "mcp",
									ContainerPort: r.getMCPPort(mcpServer),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: r.buildEnvironmentVars(mcpServer),
							// Добавить ресурсы если указаны
							Resources: r.buildResourceRequirements(mcpServer),
						},
					},
					ServiceAccountName: mcpServer.Spec.ServiceAccount,
				},
			},
		},
	}

	return deployment
}

// reconcileService создает или обновляет Service
func (r *MCPServerReconciler) reconcileService(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	service := &corev1.Service{}
	serviceName := mcpServer.Name

	err := r.Get(ctx, types.NamespacedName{
		Name:      serviceName,
		Namespace: mcpServer.Namespace,
	}, service)

	if errors.IsNotFound(err) {
		// Создаем новый Service
		service = r.createServiceForMCPServer(mcpServer)
		if err := controllerutil.SetControllerReference(mcpServer, service, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	return nil
}

// createServiceForMCPServer создает Service для MCPServer
func (r *MCPServerReconciler) createServiceForMCPServer(mcpServer *mcpv1.MCPServer) *corev1.Service {
	labels := r.labelsForMCPServer(mcpServer)
	port := r.getMCPPort(mcpServer)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "mcp",
					Port:       port,
					TargetPort: intstr.FromInt(int(port)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	return service
}

// Helper функции
func (r *MCPServerReconciler) labelsForMCPServer(mcpServer *mcpv1.MCPServer) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/name":       "mcp-server",
		"app.kubernetes.io/instance":   mcpServer.Name,
		"app.kubernetes.io/component":  "mcp-server",
		"app.kubernetes.io/created-by": "mcp-operator",
	}

	if mcpServer.Spec.Selector != nil {
		for k, v := range mcpServer.Spec.Selector {
			labels[k] = v
		}
	}

	return labels
}

func (r *MCPServerReconciler) getMCPPort(mcpServer *mcpv1.MCPServer) int32 {
	if mcpServer.Spec.Runtime.Port > 0 {
		return mcpServer.Spec.Runtime.Port
	}
	return 8080 // default port
}

func (r *MCPServerReconciler) buildEnvironmentVars(mcpServer *mcpv1.MCPServer) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	// Добавляем переменные из spec.environment
	for k, v := range mcpServer.Spec.Environment {
		envVars = append(envVars, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	// Добавляем переменные из runtime
	for k, v := range mcpServer.Spec.Runtime.Env {
		envVars = append(envVars, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	return envVars
}

func (r *MCPServerReconciler) buildResourceRequirements(mcpServer *mcpv1.MCPServer) corev1.ResourceRequirements {
	// Простая реализация - можно расширить
	return corev1.ResourceRequirements{}
}

func (r *MCPServerReconciler) updateStatus(ctx context.Context, mcpServer *mcpv1.MCPServer,
	phase mcpv1.MCPServerPhase, message, reason string) {
	mcpServer.Status.Phase = phase
	mcpServer.Status.Message = message
	mcpServer.Status.Reason = reason
	mcpServer.Status.LastUpdateTime = &metav1.Time{Time: time.Now()}
	if err := r.Status().Update(ctx, mcpServer); err != nil {
		log.FromContext(ctx).Error(err, "Failed to update MCPServer status")
	}
}

func (r *MCPServerReconciler) updateStatusFromDeployment(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      mcpServer.Name,
		Namespace: mcpServer.Namespace,
	}, deployment)

	if err != nil {
		return err
	}

	// Обновляем статус на основе состояния Deployment
	mcpServer.Status.Replicas = deployment.Status.Replicas
	mcpServer.Status.ReadyReplicas = deployment.Status.ReadyReplicas
	mcpServer.Status.AvailableReplicas = deployment.Status.AvailableReplicas

	if deployment.Status.ReadyReplicas > 0 {
		mcpServer.Status.Phase = mcpv1.MCPServerPhaseRunning
		mcpServer.Status.Message = "MCP server is running"
		mcpServer.Status.ServiceEndpoint = fmt.Sprintf("%s.%s.svc.cluster.local:%d",
			mcpServer.Name, mcpServer.Namespace, r.getMCPPort(mcpServer))
	} else if deployment.Status.Replicas == 0 {
		mcpServer.Status.Phase = mcpv1.MCPServerPhasePending
		mcpServer.Status.Message = "Waiting for deployment"
	}

	mcpServer.Status.LastUpdateTime = &metav1.Time{Time: time.Now()}
	return r.Status().Update(ctx, mcpServer)
}

// SetupWithManager sets up the controller with the Manager.
func (r *MCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1.MCPServer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	blogv1 "github.com/aghilish/operator-tutorial/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GhostReconciler reconciles a Ghost object
type GhostReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	recoder record.EventRecorder
}

const pvcNamePrefix = "ghost-data-pvc-"
const deploymentNamePrefix = "ghost-deployment-"
const svcNamePrefix = "ghost-service-"

//+kubebuilder:rbac:groups=blog.example.com,resources=ghosts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=blog.example.com,resources=ghosts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=blog.example.com,resources=ghosts/finalizers,verbs=update
//+kubebuilder:rbac:groups=blog.example.com,resources=ghosts/events,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ghost object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *GhostReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	ghost := &blogv1.Ghost{}
	if err := r.Get(ctx, req.NamespacedName, ghost); err != nil {
		log.Error(err, "Failed to get Ghost")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// Initialize completion status flags
	// Add or update the namespace first
	pvcReady := false
	deploymentReady := false
	serviceReady := false
	log.Info("Reconciling Ghost", "imageTag", ghost.Spec.ImageTag, "team", ghost.ObjectMeta.Namespace)
	// Add or update PVC
	if err := r.addPvcIfNotExists(ctx, ghost); err != nil {
		log.Error(err, "Failed to add PVC for Ghost")
		addCondition(&ghost.Status, "PVCNotReady", metav1.ConditionFalse, "PVCNotReady", "Failed to add PVC for Ghost")
		return ctrl.Result{}, err
	} else {
		pvcReady = true
	}
	// Add or update Deployment
	if err := r.addOrUpdateDeployment(ctx, ghost); err != nil {
		log.Error(err, "Failed to add or update Deployment for Ghost")
		addCondition(&ghost.Status, "DeploymentNotReady", metav1.ConditionFalse, "DeploymentNotReady", "Failed to add or update Deployment for Ghost")
		return ctrl.Result{}, err
	} else {
		deploymentReady = true
	}
	// Add or update Service
	if err := r.addServiceIfNotExists(ctx, ghost); err != nil {
		log.Error(err, "Failed to add Service for Ghost")
		addCondition(&ghost.Status, "ServiceNotReady", metav1.ConditionFalse, "ServiceNotReady", "Failed to add Service for Ghost")
		return ctrl.Result{}, err
	} else {
		serviceReady = true
	}
	// Check if all subresources are ready
	if pvcReady && deploymentReady && serviceReady {
		// Add your desired condition when all subresources are ready
		addCondition(&ghost.Status, "GhostReady", metav1.ConditionTrue, "AllSubresourcesReady", "All subresources are ready")
	}
	log.Info("Reconciliation complete")
	if err := r.updateStatus(ctx, ghost); err != nil {
		log.Error(err, "Failed to update Ghost status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func addCondition(status *blogv1.GhostStatus, condType string, statusType metav1.ConditionStatus, reason, message string) {
	for i, existingCondition := range status.Conditions {
		if existingCondition.Type == condType {
			// Condition already exists, update it
			status.Conditions[i].Status = statusType
			status.Conditions[i].Reason = reason
			status.Conditions[i].Message = message
			status.Conditions[i].LastTransitionTime = metav1.Now()
			return
		}
	}

	// Condition does not exist, add it
	condition := metav1.Condition{
		Type:               condType,
		Status:             statusType,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
	status.Conditions = append(status.Conditions, condition)
}

// Function to update the status of the Ghost object
func (r *GhostReconciler) updateStatus(ctx context.Context, ghost *blogv1.Ghost) error {
	// Update the status of the Ghost object
	if err := r.Status().Update(ctx, ghost); err != nil {
		return err
	}

	return nil
}

func (r *GhostReconciler) addPvcIfNotExists(ctx context.Context, ghost *blogv1.Ghost) error {
	log := log.FromContext(ctx)

	pvc := &corev1.PersistentVolumeClaim{}
	pvcName := pvcNamePrefix + ghost.ObjectMeta.Namespace

	err := r.Get(ctx, client.ObjectKey{Namespace: ghost.ObjectMeta.Namespace, Name: pvcName}, pvc)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return err
	}

	if err == nil {
		// PVC exists, we are done here!
		return nil
	}

	// PVC does not exist, create it
	desiredPVC := generateDesiredPVC(ghost, pvcName)
	if err := controllerutil.SetControllerReference(ghost, desiredPVC, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, desiredPVC); err != nil {
		return err
	}
	r.recoder.Event(ghost, corev1.EventTypeNormal, "PVCReady", "PVC created successfully")
	log.Info("PVC created", "pvc", pvcName)
	return nil
}

func generateDesiredPVC(ghost *blogv1.Ghost, pvcName string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: ghost.ObjectMeta.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}
}

func (r *GhostReconciler) addOrUpdateDeployment(ctx context.Context, ghost *blogv1.Ghost) error {
	log := log.FromContext(ctx)
	deploymentList := &appsv1.DeploymentList{}
	labelSelector := labels.Set{"app": "ghost-" + ghost.ObjectMeta.Namespace}

	err := r.List(ctx, deploymentList, &client.ListOptions{
		Namespace:     ghost.ObjectMeta.Namespace,
		LabelSelector: labelSelector.AsSelector(),
	})
	if err != nil {
		return err
	}

	if len(deploymentList.Items) > 0 {
		// Deployment exists, update it
		existingDeployment := &deploymentList.Items[0] // Assuming only one deployment exists
		desiredDeployment := generateDesiredDeployment(ghost)

		// Compare relevant fields to determine if an update is needed
		if existingDeployment.Spec.Template.Spec.Containers[0].Image != desiredDeployment.Spec.Template.Spec.Containers[0].Image ||
			*existingDeployment.Spec.Replicas != *desiredDeployment.Spec.Replicas {
			// Fields have changed, update the deployment
			existingDeployment.Spec = desiredDeployment.Spec
			if err := r.Update(ctx, existingDeployment); err != nil {
				return err
			}
			log.Info("Deployment updated", "deployment", existingDeployment.Name)
			r.recoder.Event(ghost, corev1.EventTypeNormal, "DeploymentUpdated", "Deployment updated successfully")
		} else {
			log.Info("Deployment is up to date, no action required", "deployment", existingDeployment.Name)
		}
		return nil
	}

	// Deployment does not exist, create it
	desiredDeployment := generateDesiredDeployment(ghost)
	if err := controllerutil.SetControllerReference(ghost, desiredDeployment, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, desiredDeployment); err != nil {
		return err
	}
	r.recoder.Event(ghost, corev1.EventTypeNormal, "DeploymentCreated", "Deployment created successfully")
	log.Info("Deployment created", "team", ghost.ObjectMeta.Namespace)
	return nil
}

func generateDesiredDeployment(ghost *blogv1.Ghost) *appsv1.Deployment {
	replicas := ghost.Spec.Replicas
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: deploymentNamePrefix,
			Namespace:    ghost.ObjectMeta.Namespace,
			Labels: map[string]string{
				"app": "ghost-" + ghost.ObjectMeta.Namespace,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "ghost-" + ghost.ObjectMeta.Namespace,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "ghost-" + ghost.ObjectMeta.Namespace,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "ghost",
							Image: "ghost:" + ghost.Spec.ImageTag,
							Env: []corev1.EnvVar{
								{
									Name:  "NODE_ENV",
									Value: "development",
								},
								{
									Name:  "database__connection__filename",
									Value: "/var/lib/ghost/content/data/ghost.db",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 2368,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "ghost-data",
									MountPath: "/var/lib/ghost/content",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "ghost-data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "ghost-data-pvc-" + ghost.ObjectMeta.Namespace,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *GhostReconciler) addServiceIfNotExists(ctx context.Context, ghost *blogv1.Ghost) error {
	log := log.FromContext(ctx)
	service := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{Namespace: ghost.ObjectMeta.Namespace, Name: svcNamePrefix + ghost.ObjectMeta.Namespace}, service)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return err
	}

	if err == nil {
		// Service exists
		return nil
	}
	// Service does not exist, create it
	desiredService := generateDesiredService(ghost)
	if err := controllerutil.SetControllerReference(ghost, desiredService, r.Scheme); err != nil {
		return err
	}

	// Service does not exist, create it
	if err := r.Create(ctx, desiredService); err != nil {
		return err
	}
	r.recoder.Event(ghost, corev1.EventTypeNormal, "ServiceCreated", "Service created successfully")
	log.Info("Service created", "service", desiredService.Name)
	return nil
}

func generateDesiredService(ghost *blogv1.Ghost) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ghost-service-" + ghost.ObjectMeta.Namespace,
			Namespace: ghost.ObjectMeta.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(2368),
					NodePort:   30001,
				},
			},
			Selector: map[string]string{
				"app": "ghost-" + ghost.ObjectMeta.Namespace,
			},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *GhostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recoder = mgr.GetEventRecorderFor("ghost-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&blogv1.Ghost{}).
		Complete(r)
}

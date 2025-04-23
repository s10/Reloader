package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stakater/Reloader/internal/pkg/options"
	"github.com/stakater/Reloader/pkg/kube"
	app "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	patchtypes "k8s.io/apimachinery/pkg/types"
)

func pauseDeployment(deployment *app.Deployment, clients kube.Clients, namespace, pauseIntervalValue string) error {
	pauseDuration, err := parsePauseDuration(pauseIntervalValue)
	if err != nil {
		return err
	}

	deploymentName := deployment.Name

	if !deployment.Spec.Paused {
		logrus.Infof("Pausing Deployment '%s' in namespace '%s' for %s", deploymentName, namespace, pauseDuration)

		deploymentFuncs := GetDeploymentRollingUpgradeFuncs()

		pausePatch, err := createPausePatch()
		if err != nil {
			logrus.Errorf("Failed to create pause patch for deployment '%s': %v", deploymentName, err)
			return err
		}

		err = deploymentFuncs.PatchFunc(clients, namespace, deployment, patchtypes.StrategicMergePatchType, pausePatch)

		if err != nil {
			logrus.Errorf("Failed to patch deployment '%s' in namespace '%s': %v", deploymentName, namespace, err)
			return err
		}

		createResumeTimer(deployment, clients, namespace, pauseDuration)
	} else {
		logrus.Infof("Deployment '%s' in namespace '%s' is already paused", deploymentName, namespace)
	}
	return nil
}

func createResumeTimer(deployment *app.Deployment, clients kube.Clients, namespace string, pauseDuration time.Duration) {
	time.AfterFunc(pauseDuration, func() {
		resumeDeployment(deployment, namespace, clients)
	})
}

func resumeDeployment(deployment *app.Deployment, namespace string, clients kube.Clients) {
	deploymentName := deployment.Name

	currentDeployment, err := clients.KubernetesClient.AppsV1().Deployments(namespace).Get(context.TODO(), deploymentName, metav1.GetOptions{})
	if err != nil {
		logrus.Errorf("Failed to get deployment '%s' in namespace '%s': %v", deploymentName, namespace, err)
		return
	}

	if !currentDeployment.Spec.Paused {
		logrus.Infof("Deployment '%s' in namespace '%s' not paused. Skipping resume", deploymentName, namespace)
		return
	}

	pausedAtAnnotationValue := currentDeployment.Annotations[options.PauseDeploymentTimeAnnotation]
	if pausedAtAnnotationValue == "" {
		logrus.Infof("Deployment '%s' in namespace '%s' was not paused by Reloader. Skipping resume", deploymentName, namespace)
		return
	}

	deploymentFuncs := GetDeploymentRollingUpgradeFuncs()

	resumePatch, err := createResumePatch()
	if err != nil {
		logrus.Errorf("Failed to create resume patch for deployment '%s': %v", deploymentName, err)
		return
	}

	err = deploymentFuncs.PatchFunc(clients, namespace, currentDeployment, patchtypes.StrategicMergePatchType, resumePatch)

	if err != nil {
		logrus.Errorf("Failed to resume deployment '%s' in namespace '%s': %v", deploymentName, namespace, err)
		return
	}

	logrus.Infof("Successfully resumed deployment '%s' in namespace '%s'", deploymentName, namespace)
}

func parsePauseDuration(pauseIntervalValue string) (time.Duration, error) {
	pauseDuration, err := time.ParseDuration(pauseIntervalValue)
	if err != nil {
		logrus.Warnf("Failed to parse pause interval value '%s': %v", pauseIntervalValue, err)
		return 0, err
	}
	return pauseDuration, nil
}

func createPausePatch() ([]byte, error) {
	patchData := map[string]interface{}{
		"spec": map[string]interface{}{
			"paused": true,
		},
		"metadata": map[string]interface{}{
			"annotations": map[string]string{
				options.PauseDeploymentTimeAnnotation: time.Now().Format(time.RFC3339),
			},
		},
	}

	return json.Marshal(patchData)
}

func createResumePatch() ([]byte, error) {
	patchData := map[string]interface{}{
		"spec": map[string]interface{}{
			"paused": false,
		},
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				options.PauseDeploymentTimeAnnotation: nil,
			},
		},
	}

	return json.Marshal(patchData)
}

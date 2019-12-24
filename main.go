package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/katainaka0503/berglas-aws/pkg/resolution"
	kwhhttp "github.com/slok/kubewebhook/pkg/http"
	kwhlog "github.com/slok/kubewebhook/pkg/log"
	kwhmutating "github.com/slok/kubewebhook/pkg/webhook/mutating"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// berglasAWSContainer is the default berglas-aws container from which to pull the
	// berglas-aws binary.
	berglasAWSContainer = "katainaka0503/berglas-aws:latest"

	// binVolumeName is the name of the volume where the berglas-aws binary is stored.
	binVolumeName = "berglas-aws-bin"

	// binVolumeMountPath is the mount path where the berglas-aws binary can be found.
	binVolumeMountPath = "/berglas-aws/bin"
)

// binInitContainer is the container that pulls the berglas-aws binary executable
// into a shared volume mount.
var binInitContainer = corev1.Container{
	Name:            "copy-berglas-aws-bin",
	Image:           berglasAWSContainer,
	ImagePullPolicy: corev1.PullIfNotPresent,
	Command: []string{"sh", "-c",
		fmt.Sprintf("cp $(which berglas-aws) %s", binVolumeMountPath)},
	VolumeMounts: []corev1.VolumeMount{
		{
			Name:      binVolumeName,
			MountPath: binVolumeMountPath,
		},
	},
}

// binVolume is the shared, in-memory volume where the berglas binary lives.
var binVolume = corev1.Volume{
	Name: binVolumeName,
	VolumeSource: corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{
			Medium: corev1.StorageMediumMemory,
		},
	},
}

// binVolumeMount is the shared volume mount where the berglas binary lives.
var binVolumeMount = corev1.VolumeMount{
	Name:      binVolumeName,
	MountPath: binVolumeMountPath,
	ReadOnly:  true,
}


type BerglasAWSMutator struct {
	logger kwhlog.Logger
}

func (m *BerglasAWSMutator) Mutate(ctx context.Context, obj metav1.Object) (bool, error) {
	m.logger.Infof("calling mutate")

	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return false, nil
	}

	mutated := false

	for i, c := range pod.Spec.InitContainers {
		c, didMutate := m.mutateContainer(ctx, &c)
		if didMutate {
			mutated = true
			pod.Spec.InitContainers[i] = *c
		}
	}

	for i, c := range pod.Spec.Containers {
		c, didMutate := m.mutateContainer(ctx, &c)
		if didMutate {
			mutated = true
			pod.Spec.Containers[i] = *c
		}
	}

	// If any of the containers requested berglas secrets, mount the shared volume
	// and ensure the berglas binary is available via an init container.
	if mutated {
		pod.Spec.Volumes = append(pod.Spec.Volumes, binVolume)
		pod.Spec.InitContainers = append([]corev1.Container{binInitContainer},
			pod.Spec.InitContainers...)
	}

	return false, nil
}

func (m *BerglasAWSMutator) mutateContainer(_ context.Context, c *corev1.Container) (*corev1.Container, bool) {
	// Ignore if there are no berglas references in the container.
	if !m.hasBerglasAWSReferences(c.Env) {
		return c, false
	}

	// Berglas-AWS prepends the command from the podspec. If there's no command in the
	// podspec, there's nothing to append. Note: this is the command in the
	// podspec, not a CMD or ENTRYPOINT in a Dockerfile.
	if len(c.Command) == 0 {
		m.logger.Warningf("cannot apply berglas to %s: container spec does not define a command", c.Name)
		return c, false
	}

	// Add the shared volume mount
	c.VolumeMounts = append(c.VolumeMounts, binVolumeMount)

	// Prepend the command with berglas-aws exec --
	original := append(c.Command, c.Args...)
	c.Command = []string{binVolumeMountPath + "berglas-aws"}
	c.Args = append([]string{"exec", "--"}, original...)

	return c, true
}

func (m *BerglasAWSMutator) hasBerglasAWSReferences(env []corev1.EnvVar) bool {
	for _, e := range env {
		if resolution.IsResolvable(e.Value){
			return true
		}
	}
	return false
}

func main() {
	config := parseConfig()

	logger := &kwhlog.Std{Debug: true}

	mutator := &BerglasAWSMutator{
		logger: logger,
	}

	webhookConfig := kwhmutating.WebhookConfig{
		Name: "datadogSidecarInjection",
		Obj:  &corev1.Pod{},
	}

	webhook, err := kwhmutating.NewWebhook(webhookConfig, mutator, nil, nil, logger)
	if err != nil {
		logger.Errorf("error creating webhook: %s", err)
		os.Exit(1)
	}

	handler, err := kwhhttp.HandlerFor(webhook)
	if err != nil {
		logger.Errorf("error serving webhook: %s", err)
		os.Exit(1)
	}

	address := fmt.Sprintf(":%v", config.port)
	logger.Infof("Listening on %v", address)
	err = http.ListenAndServeTLS(address, config.certFile, config.keyFile, handler)
	if err != nil {
		logger.Errorf("error serving webhook: %s", err)
		os.Exit(1)
	}
}

type Config struct {
	port int
	certFile string
	keyFile string
}

func parseConfig() Config{
	var config Config
	flag.IntVar(&config.port, "port", 443, "Webhook server port.")
	flag.StringVar(&config.certFile, "tls-cert-file", "/etc/webhook/certs/cert.pem", "File containing the x509 Certificate for HTTPS.")
	flag.StringVar(&config.keyFile, "tls-key-file", "/etc/webhook/certs/key.pem", "File containing the x509 private key to --tls-key-file.")
	flag.Parse()

	return config
}

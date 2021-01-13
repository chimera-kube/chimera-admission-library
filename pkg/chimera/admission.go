package chimera

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client/config"
)

type WebhookCallback func(*admissionv1.AdmissionRequest) (WebhookResponse, error)

type Webhook struct {
	Rules         []admissionregistrationv1.RuleWithOperations
	Callback      WebhookCallback
	FailurePolicy admissionregistrationv1.FailurePolicyType // +optional
	Name          string                                    // +optional
	Path          string                                    // +optional
}

type WebhookList []Webhook

func internalServerError(log Logger, w http.ResponseWriter, err error) {
	log.Errorf("Internal server error: %v", err)
	w.WriteHeader(http.StatusInternalServerError)
}

func performValidation(callback WebhookCallback, log Logger, w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	log.Debugf("Validating request: %s", string(body))
	admissionReview := admissionv1.AdmissionReview{}
	err := json.Unmarshal(body, &admissionReview)
	if err != nil {
		internalServerError(log, w, err)
		return
	}
	webhookResponse, err := callback(admissionReview.Request)
	if err != nil {
		internalServerError(log, w, err)
		return
	}
	admissionResponse := admissionv1.AdmissionResponse{
		UID:     admissionReview.Request.UID,
		Allowed: webhookResponse.Allowed,
		Result:  &metav1.Status{},
	}
	if webhookResponse.Code != nil {
		admissionResponse.Result.Code = *webhookResponse.Code
	}
	if webhookResponse.RejectionMessage != nil {
		admissionResponse.Result.Message = *webhookResponse.RejectionMessage
	}
	admissionReview.Response = &admissionResponse
	marshaledAdmissionReview, err := json.Marshal(admissionReview)
	if err != nil {
		internalServerError(log, w, err)
		return
	}
	log.Debugf("Validation response: %s", string(marshaledAdmissionReview))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(marshaledAdmissionReview)
}

func (webhooks WebhookList) asValidatingAdmissionRegistration(admissionConfig *AdmissionConfig, caBundle []byte) admissionregistrationv1.ValidatingWebhookConfiguration {
	res := admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: admissionConfig.Name,
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{},
	}
	sideEffects := admissionregistrationv1.SideEffectClassNone
	for i := 0; i < len(admissionConfig.Webhooks); i++ {
		webhook := admissionConfig.Webhooks[i]
		webhookPath := webhook.Path
		admissionCallbackURL := url.URL{
			Scheme: "https",
			Host: net.JoinHostPort(
				admissionConfig.CallbackHost,
				strconv.Itoa(admissionConfig.CallbackPort)),
			Path: webhook.Path,
		}
		admissionCallback := admissionCallbackURL.String()

		clientConfig := admissionregistrationv1.WebhookClientConfig{
			CABundle: caBundle,
		}
		if admissionConfig.KubeNamespace != "" && admissionConfig.KubeService != "" {
			port := int32(admissionConfig.CallbackPort)
			clientConfig.Service = &admissionregistrationv1.ServiceReference{
				Namespace: admissionConfig.KubeNamespace,
				Name:      admissionConfig.KubeService,
				Path:      &webhookPath,
				Port:      &port,
			}
		} else {
			clientConfig.URL = &admissionCallback
		}

		validatingWebhook := admissionregistrationv1.ValidatingWebhook{
			Name:                    webhook.Name,
			ClientConfig:            clientConfig,
			Rules:                   webhook.Rules,
			SideEffects:             &sideEffects,
			AdmissionReviewVersions: []string{"v1"},
		}
		if validatingWebhook.Name == "" {
			validatingWebhook.Name = fmt.Sprintf("rule-%d", i)
		}
		if webhook.FailurePolicy == "" {
			validatingWebhook.FailurePolicy = nil
		} else {
			validatingWebhook.FailurePolicy = &webhook.FailurePolicy
		}
		validatingWebhook.Name = fmt.Sprintf(
			"%s.%s",
			validatingWebhook.Name,
			admissionConfig.Name)
		res.Webhooks = append(res.Webhooks, validatingWebhook)
	}
	return res
}

func setupAdmissionWebhooks(admissionConfig *AdmissionConfig) {
	var log Logger
	if admissionConfig.Log == nil {
		log = &simpleLogger{}
	} else {
		log = admissionConfig.Log
	}

	for _, webhook := range admissionConfig.Webhooks {
		if webhook.Path == "" {
			webhook.Path = generateValidatePath()
		}
		http.HandleFunc(webhook.Path, func(w http.ResponseWriter, r *http.Request) {
			performValidation(webhook.Callback, log, w, r)
		})
	}
}

func registerAdmissionWebhooks(admissionConfig *AdmissionConfig, caCertificate []byte) error {
	var log Logger
	if admissionConfig.Log == nil {
		log = &simpleLogger{}
	} else {
		log = admissionConfig.Log
	}

	kubeCfg, err := kubeclient.GetConfig()
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(kubeCfg)
	if err != nil {
		return err
	}
	caBundle, err := pemEncodeCertificate(caCertificate)
	if err != nil {
		return err
	}
	webhookCfg := admissionConfig.Webhooks.asValidatingAdmissionRegistration(admissionConfig, caBundle)
	for {
		err := clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(
			context.TODO(),
			admissionConfig.Name,
			metav1.DeleteOptions{},
		)
		if err != nil && !apierrors.IsNotFound(err) {
			log.Errorf("Could not cleanup webhook prior to start: %v", err)
		}
		webhookList, err := clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(
			context.TODO(),
			metav1.ListOptions{},
		)
		if err == nil {
			if len(webhookList.Items) != 0 {
				log.Infof("WARNING: there are %d webhook(s) already registered besides this admission that could reject requests:\n", len(webhookList.Items))
				for _, webhook := range webhookList.Items {
					log.Debugf("  - %s\n", webhook.ObjectMeta.Name)
				}
			}
		} else {
			log.Errorf("Could not list current validation webhooks: %v\n", err)
		}
		_, err = clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(
			context.TODO(),
			&webhookCfg,
			metav1.CreateOptions{},
		)
		if err == nil {
			log.Infof(
				"webhook for admission %q correctly installed -- %d hook(s) active for this admission",
				admissionConfig.Name,
				len(admissionConfig.Webhooks))
			break
		}
		log.Errorf("could not register webhook: %v", err)
	}
	return nil
}

func generateValidatePath() string {
	return fmt.Sprintf("/validate-%s", uuid.New().String())
}

type AdmissionConfig struct {
	Name                      string
	KubeNamespace             string
	KubeService               string
	CallbackHost              string
	CallbackPort              int
	Webhooks                  WebhookList
	TLSExtraSANs              []string
	CertFile                  string
	KeyFile                   string
	CaFile                    string
	SkipAdmissionRegistration bool
	Log                       Logger
}

func StartServer(config *AdmissionConfig, insecure bool) error {
	if config.CallbackHost == "" {
		config.CallbackHost = "localhost"
	}

	var caCertFile, certFile, keyFile string
	if config.CertFile != "" && config.KeyFile != "" {
		certFile = config.CertFile
		keyFile = config.KeyFile
		caCertFile = config.CaFile
	} else {
		var err error
		caCertFile, certFile, keyFile, err = automaticCertGeneration(
			config.CallbackHost,
			config.TLSExtraSANs)

		if err != nil {
			return err
		}
		defer os.Remove(caCertFile)
		defer os.Remove(keyFile)
		defer os.Remove(certFile)
	}

	setupAdmissionWebhooks(config)

	if !config.SkipAdmissionRegistration {
		caBundle, err := ioutil.ReadFile(caCertFile)
		if err != nil {
			return err
		}
		if err := registerAdmissionWebhooks(config, caBundle); err != nil {
			return err
		}
	}

	if insecure {
		fmt.Printf("Starting HTTP server on :%d\n", config.CallbackPort)
		return http.ListenAndServe(fmt.Sprintf(":%d", config.CallbackPort), nil)
	} else {
		fmt.Printf("Starting TLS server on :%d - using key: %s, cert %s, CABundle %s\n",
			config.CallbackPort, keyFile, certFile, caCertFile)

		return http.ListenAndServeTLS(fmt.Sprintf(":%d", config.CallbackPort), certFile, keyFile, nil)
	}
}

func automaticCertGeneration(callbackHost string, extraSANs []string) (string, string, string, error) {
	caCert, CAPrivateKey, err := generateCA()
	if err != nil {
		return "", "", "", errors.Errorf("failed to generate CA certificate: %v", err)
	}

	servingCert, servingKey, err := generateCert(
		caCert,
		callbackHost,
		extraSANs,
		CAPrivateKey.Key())
	if err != nil {
		return "", "", "", errors.Errorf("failed to generate serving certificate: %v", err)
	}

	caCertFile, err := ioutil.TempFile("", "validating-webhook-ca*.crt")
	if err != nil {
		return "", "", "", err
	}
	certFile, err := ioutil.TempFile("", "validating-webhook-*.crt")
	if err != nil {
		defer os.Remove(caCertFile.Name())
		return "", "", "", err
	}
	keyFile, err := ioutil.TempFile("", "validating-webhook-*.key")
	if err != nil {
		defer os.Remove(caCertFile.Name())
		defer os.Remove(certFile.Name())
		return "", "", "", err
	}

	if err := ioutil.WriteFile(caCertFile.Name(), caCert, 0644); err != nil {
		defer os.Remove(caCertFile.Name())
		defer os.Remove(certFile.Name())
		defer os.Remove(keyFile.Name())
		return "", "", "", err
	}

	if err := ioutil.WriteFile(certFile.Name(), servingCert, 0644); err != nil {
		defer os.Remove(caCertFile.Name())
		defer os.Remove(certFile.Name())
		defer os.Remove(keyFile.Name())
		return "", "", "", err
	}
	if err := ioutil.WriteFile(keyFile.Name(), servingKey, 0600); err != nil {
		defer os.Remove(caCertFile.Name())
		defer os.Remove(certFile.Name())
		defer os.Remove(keyFile.Name())
		return "", "", "", err
	}

	return caCertFile.Name(), certFile.Name(), keyFile.Name(), nil

}

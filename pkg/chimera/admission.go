package chimera

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type WebhookCallback func(*admissionv1.AdmissionRequest) (WebhookResponse, error)

type Webhook struct {
	Rules    []admissionregistrationv1.RuleWithOperations
	Callback WebhookCallback
	Path     string
}

func internalServerError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	log.Printf(">>> (500) %v\n", err)
}

func performValidation(callback WebhookCallback, w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	log.Printf("<<< %s", string(body))
	admissionReview := admissionv1.AdmissionReview{}
	err := json.Unmarshal(body, &admissionReview)
	if err != nil {
		internalServerError(w, err)
		return
	}
	webhookResponse, err := callback(admissionReview.Request)
	if err != nil {
		internalServerError(w, err)
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
		internalServerError(w, err)
		return
	}
	log.Printf(">>> (200) %s", string(marshaledAdmissionReview))
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write(marshaledAdmissionReview)
}

func registerAdmissionWebhook(webhookName, admissionValidatePath, callbackHost string, callbackPort int, rules []admissionregistrationv1.RuleWithOperations, caCertificate []byte) error {
	config, err := config.GetConfig()
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	caBundle, err := pemEncodeCertificate(caCertificate)
	if err != nil {
		return err
	}
	admissionCallbackURL := url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(callbackHost, strconv.Itoa(callbackPort)),
		Path:   admissionValidatePath,
	}
	admissionCallback := admissionCallbackURL.String()
	sideEffects := admissionregistrationv1.SideEffectClassNone
	admissionConfig := admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: webhookName,
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name: webhookName,
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					URL:      &admissionCallback,
					CABundle: caBundle,
				},
				Rules:                   rules,
				SideEffects:             &sideEffects,
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}
	for {
		err := clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(
			context.TODO(),
			webhookName,
			metav1.DeleteOptions{},
		)
		if err != nil {
			log.Printf("could not unregister webhook: %v", err)
		}
		_, err = clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(
			context.TODO(),
			&admissionConfig,
			metav1.CreateOptions{},
		)
		if err == nil {
			break
		}
		log.Printf("could not register webhook: %v", err)
	}
	return nil
}

func generateAdmissionValidatePath() string {
	return fmt.Sprintf("/validate-%s", uuid.New().String())
}

func StartServer(webhookName, callbackHost string, callbackPort int, webhooks []Webhook) error {
	caCert, CAPrivateKey, err := generateCA()
	if err != nil {
		return errors.Errorf("failed to generate CA certificate: %v", err)
	}
	servingCert, servingKey, err := generateCert(caCert, []string{callbackHost}, CAPrivateKey.Key())
	if err != nil {
		return errors.Errorf("failed to generate serving certificate: %v", err)
	}
	for _, webhook := range webhooks {
		admissionValidatePath := webhook.Path
		if admissionValidatePath == "" {
			admissionValidatePath = generateAdmissionValidatePath()
		}
		if err := registerAdmissionWebhook(webhookName, admissionValidatePath, callbackHost, callbackPort, webhook.Rules, caCert); err != nil {
			return errors.Errorf("failed to register admission webhook: %v", err)
		}
		http.HandleFunc(admissionValidatePath, func(w http.ResponseWriter, r *http.Request) {
			performValidation(webhook.Callback, w, r)
		})
	}
	certFile, err := ioutil.TempFile("", "validating-webhook-*.crt")
	if err != nil {
		return err
	}
	keyFile, err := ioutil.TempFile("", "validating-webhook-*.key")
	if err != nil {
		return err
	}
	defer os.Remove(keyFile.Name())
	defer os.Remove(certFile.Name())
	if err := ioutil.WriteFile(certFile.Name(), servingCert, 0644); err != nil {
		return err
	}
	if err := ioutil.WriteFile(keyFile.Name(), servingKey, 0600); err != nil {
		return err
	}
	return http.ListenAndServeTLS(fmt.Sprintf(":%d", callbackPort), certFile.Name(), keyFile.Name(), nil)
}

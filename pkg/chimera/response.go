package chimera

import (
	admissionv1 "k8s.io/api/admission/v1"
)

type WebhookResponse struct {
	Allowed          bool
	Code             *int32
	RejectionMessage *string
}

func NewAllowRequest() WebhookResponse {
	return WebhookResponse{
		Allowed: true,
	}
}

func AllowRequest(*admissionv1.AdmissionRequest) (WebhookResponse, error) {
	return NewAllowRequest(), nil
}

func NewRejectRequest() WebhookResponse {
	return WebhookResponse{
		Allowed: false,
	}
}

func RejectRequest(*admissionv1.AdmissionRequest) (WebhookResponse, error) {
	return NewRejectRequest(), nil
}

func (r WebhookResponse) WithCode(code int32) WebhookResponse {
	if !r.Allowed {
		r.Code = &code
	}
	return r
}

func (r WebhookResponse) WithMessage(message string) WebhookResponse {
	if !r.Allowed {
		r.RejectionMessage = &message
	}
	return r
}

# chimera

`chimera` is a library that allows you to trivially write admission
webhooks that will self-register against a Kubernetes cluster,
specially for testing purposes.

Given you want to test some webhook logic, you can write a simple file
that will start a webhook server calling to your declared callbacks,
creating the required certificates and registering the webhook against
a Kubernetes cluster.

You can check some examples in the [chimera-samples github
project](https://github.com/chimera-kube/chimera-samples).


## How it works

You need a `kubeconfig` file that has permissions to delete and create
`ValidatingWebhookConfiguration` resources. By default, when you start
the `chimera` server from your program, `chimera` will look for your
Kubernetes configuration using the following precedence rules:

- (TODO) Provided `kubeconfig` path, if any
- `KUBECONFIG` environment variable
- `$HOME/.kube/config` if exists

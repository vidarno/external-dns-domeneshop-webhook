# Domeneshop Webhook for external-dns

ExternalDNS is a Kubernetes add-on for automatically managing Domain Name System (DNS) records for Kubernetes services by using different DNS providers. By default, Kubernetes manages DNS records internally, but ExternalDNS takes this functionality a step further by delegating the management of DNS records to an external DNS provider such as Domeneshop. Therefore, the Domeneshop webhook allows to manage your Domeneshop domains inside your kubernetes cluster with ExternalDNS.

To use ExternalDNS with Domeneshop, you need to create a token and a secret for the account managing your domains. See the Domeneshop API documentation for details - https://api.domeneshop.no/docs/


# Kubernetes Deployment

kubectl create secret generic external-dns-domeneshop-webhook \
  --from-literal=TOKEN=value1 \
  --from-literal=SECRET=value2

Install external-dns and use values-file to configure domeneshop-webhook as sidecar:

helm install external-dns oci://registry-1.docker.io/bitnamicharts/external-dns -f external-dns-domeneshop-webhook-values.yaml

If you want external-dns to also allow deletion of records add --set policy=sync:

helm install external-dns oci://registry-1.docker.io/bitnamicharts/external-dns -f external-dns-domeneshop-webhook-values.yaml --set policy=sync

This repository targets `external-dns` `v0.21.0` and the webhook reads optional domain-filter configuration from environment variables. The following variables are supported:

- `DOMAIN_FILTER` or `DOMAIN_FILTER_INCLUDE`: comma-separated allowlist of domains
- `DOMAIN_FILTER_EXCLUDE`: comma-separated denylist of domains
- `DOMAIN_FILTER_REGEX_INCLUDE`: regex used to include matching domains
- `DOMAIN_FILTER_REGEX_EXCLUDE`: regex used to exclude matching domains

Example values for the sidecar environment:

```yaml
env:
  - name: DOMAIN_FILTER_INCLUDE
    value: example.com,example.org
  - name: DOMAIN_FILTER_EXCLUDE
    value: internal.example.com
  - name: DOMAIN_FILTER_REGEX_INCLUDE
    value: '.*\\.example\\.com$'
```

# Domeneshop API

The Domeneshop API client is based on cert-manager-webhook-domeneshop made by Domeneshop, but extended with helper-functions and support for records other than TXT-records.

Domeneshop enforces the RFCs (RFC 1034 section 3.6.2, RFC 1912 section 2.4), it not permissible for a CNAME record to co-exist with any other records, even TXT records. Using --txt-prefix might be a workaround (https://github.com/kubernetes-sigs/external-dns/issues/262)

# Design

main.go - Base application, starting webserver and adding routes

pkg/webhook/webhook.go - Routes for webserver, uses provider-package to talk to Domeneshop API via domeneshop client-package

internal/client/domeneshop.go - client for Domeneshop API

internal/provider/domeneshop.go - Functions that use Domeneshop API for calls from the webserver-routes

# Development

While developing the webhook, point external-dns to the Docker gateway IP-address on the host ( 172.17.0.1 )

Install external-dns via Helm: 

  helm upgrade my-release oci://registry-1.docker.io/bitnamicharts/external-dns

Edit deployment to pass these args to use a locally-running webhook:

- --provider=webhook
- --webhook-provider-url=http://172.17.0.1:8888

Might be useful:

webhook-provider-read-timeout

webhook-provider-write-timeout

Webhook documentation: https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/webhook-provider.md

Domeneshop API documentation: https://api.domeneshop.no/docs/

Domeneshop cert-manager webhook: https://github.com/domeneshop/cert-manager-webhook-domeneshop
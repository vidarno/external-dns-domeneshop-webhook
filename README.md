
 * Domeneshop API client is based on cert-manager-webhook-domeneshop made by Domeneshop, but extended with helper-functions and support for A- and AAAA-records

TODO
* Context-aware logging
* Support dry-run
* Remove uncommented code (but keep actual comments, maybe)
* Handle "duplicate" records (like redundant MX-records)
* MX-records - https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/mx-record.md
* Handle changing records (new A-record or change between A and CNAME)

Good to know
* Domeneshop enforces the RFCs (RFC 1034 section 3.6.2, RFC 1912 section 2.4), it not permissible for a CNAME record to co-exist with any other records, even TXT records. Using --txt-prefix might be a workaround (https://github.com/kubernetes-sigs/external-dns/issues/262)

Design
main.go - Base application, starting webserver and adding routes
pkg/webhook/webhook.go - Routes for webserver, uses provider-package to talk to Domeneshop API via domeneshop client-package
internal/client/domeneshop.go - client for Domeneshop API
internal/provider/domeneshop.go - Functions that use Domeneshop API for calls from the webserver-routes

Development
While developing the webhook, point external-dns to the Docker gateway IP-address on the host ( 172.17.0.1 )
Install external-dns via Helm: helm install my-release oci://registry-1.docker.io/bitnamicharts/external-dns
Edit deployment to pass these args to use a locally-running webhook:
- --provider=webhook
- --webhook-provider-url=http://172.17.0.1:8888
Might be useful:
webhook-provider-read-timeout
webhook-provider-write-timeout

Webhook documentation: https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/webhook-provider.md
Domeneshop API documentation: https://api.domeneshop.no/docs/
Domeneshop cert-manager webhook: https://github.com/domeneshop/cert-manager-webhook-domeneshop
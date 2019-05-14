package handler

import (
	"fmt"
	"strings"

	"github.com/go-acme/lego/certificate"
	"github.com/go-acme/lego/lego"
	"github.com/go-acme/lego/registration"
	"github.com/pkg/errors"

	"github.com/begmaroman/acme-dns-route53/handler/r53dns"
)

var (
	// registerOptions is the predefined registration.RegisterOptions struct with the default params
	registerOptions = registration.RegisterOptions{TermsOfServiceAgreed: true}
)

// Obtain creates a new SSL certificate or renews existing one for the given domains with the given email
func (h *CertificateHandler) Obtain(domains []string, email string) error {
	// Load user
	certUser, err := getUser(h.toUserParams(email))
	if err != nil {
		return errors.Wrap(err, "unable to load user")
	}

	// Create config
	config, err := getConfig(h.toConfigParams(certUser))
	if err != nil {
		return errors.Wrap(err, "unable to create config")
	}

	// Create a client facilitates communication with the CA server.
	client, err := lego.NewClient(config)
	if err != nil {
		return errors.Wrap(err, "unable to create lego client")
	}

	// Use DNS-01 challenge to verify that the given domain belongs to the current server
	if err = client.Challenge.SetDNS01Provider(r53dns.NewProvider(h.r53, h.log)); err != nil {
		return errors.Wrap(err, "failed to set DNS-01 provider")
	}

	// New users will need to register
	if certUser.Registration, err = client.Registration.Register(registerOptions); err != nil {
		return errors.Wrap(err, "could not register Let's Encrypt account")
	}

	// Create a new request to obtain certificate
	crt, err := client.Certificate.Obtain(certificate.ObtainRequest{
		Domains:    domains,
		PrivateKey: nil,
		Bundle:     false,
		MustStaple: false,
	})
	if err != nil {
		return errors.Wrap(err, "unable to obtain certificate")
	}

	// Store the obtained certificate
	if err := h.store.Store(crt, domains); err != nil {
		return errors.Wrap(err, "unable to store certificates")
	}

	// Notify that the certificate has been obtained for the given domains
	if len(h.notificationTopic) > 0 {
		if err := h.notifier.Notify(h.notificationTopic, fmt.Sprintf("[%s] certificate successfully obtained", strings.Join(domains, ", "))); err != nil {
			h.log.WithError(err).Error("failed to publish notification")
		}
	}

	// Store user's private key into config file by the config path
	if err := certUser.StorePrivateKey(h.configDir); err != nil {
		h.log.WithError(err).Error("unable to store user's private key")
	}

	return nil
}

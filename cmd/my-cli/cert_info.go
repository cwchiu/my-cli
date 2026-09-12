package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cwchiu/my-cli/internal/certinfo"
	"github.com/spf13/cobra"
)

// defaultCertTimeout bounds one cert-info run (dial + handshake).
const defaultCertTimeout = 10 * time.Second

// certExpiryWarnDays is the remaining-days threshold below which the table
// output flags the certificate as expiring soon.
const certExpiryWarnDays = 30

// certConfig holds the resolved settings for one cert-info run.
type certConfig struct {
	target       string
	insecure     bool
	timeout      time.Duration
	outputFormat string
	outFile      string
}

// newCertInfoCmd returns the `cert-info` subcommand, which connects to a
// TLS endpoint and prints the served certificate chain in human-readable
// form (issue #10).
func newCertInfoCmd() *cobra.Command {
	var raw certConfig

	cmd := &cobra.Command{
		Use:   "cert-info <host[:port]>",
		Short: "Show the TLS certificate chain served by a host",
		Long: `Connect to host[:port] over TLS (default port 443), perform a
handshake, and print the served certificate chain in human-readable form:
subject, issuer, validity window, SANs, signature and key algorithms,
SHA-256 fingerprint, and the negotiated TLS version and cipher.

Use --insecure to inspect servers with self-signed or internal-CA
certificates that would otherwise fail verification.

Output formats:
  table  human-readable summary (default)
  json   machine-readable certificate details`,
		Example: `  my-cli cert-info example.com
  my-cli cert-info example.com:8443
  my-cli cert-info internal.example.com --insecure
  my-cli cert-info example.com --output json`,
		Args: wrapUsage(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveCertConfig(raw, args)
			if err != nil {
				return fmt.Errorf("%w: %w", errUsage, err)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()

			info, err := certinfo.Fetch(ctx, cfg.target, certinfo.Options{
				Insecure: cfg.insecure,
				Timeout:  cfg.timeout,
			})
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()

			if cfg.outFile != "" {
				f, ferr := os.Create(cfg.outFile)
				if ferr != nil {
					return fmt.Errorf("create output file: %w", ferr)
				}

				defer func() {
					_ = f.Close()
				}()

				out = f
			}

			return renderCertInfo(out, cfg.outputFormat, info)
		},
	}

	cmd.Flags().BoolVar(&raw.insecure, "insecure", false,
		"skip certificate verification (for self-signed or internal CA chains)")
	cmd.Flags().DurationVar(&raw.timeout, "timeout", defaultCertTimeout,
		"connect and handshake timeout")
	cmd.Flags().StringVarP(&raw.outputFormat, "output", "o", outputFormatTable,
		"output format (table|json)")
	cmd.Flags().StringVarP(&raw.outFile, "out", "O", "",
		"write the output to this file instead of stdout")

	_ = cmd.RegisterFlagCompletionFunc("output", func(
		_ *cobra.Command, _ []string, _ string,
	) ([]string, cobra.ShellCompDirective) {
		return []string{outputFormatTable, outputFormatJSON}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

// resolveCertConfig validates the raw flag values, failing fast before any
// network I/O.
func resolveCertConfig(raw certConfig, args []string) (certConfig, error) {
	if raw.timeout <= 0 {
		return certConfig{}, fmt.Errorf("timeout must be positive, got %s", raw.timeout)
	}

	switch raw.outputFormat {
	case outputFormatTable, outputFormatJSON:
	default:
		return certConfig{}, fmt.Errorf("unsupported output format %q", raw.outputFormat)
	}

	if len(args) != 1 {
		return certConfig{}, fmt.Errorf("expected exactly one target, got %d", len(args))
	}

	raw.target = strings.TrimSpace(args[0])
	if raw.target == "" {
		return certConfig{}, errors.New("target must not be empty")
	}

	return raw, nil
}

// renderCertInfo writes the certificate info in the requested format.
func renderCertInfo(out io.Writer, format string, info *certinfo.Info) error {
	switch format {
	case outputFormatTable:
		return renderCertTable(out, info)
	case outputFormatJSON:
		return renderJSON(out, info)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

// renderJSON writes the info as indented JSON.
func renderJSON(out io.Writer, info *certinfo.Info) error {
	data, err := jsonMarshalIndent(info)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, string(data)); err != nil {
		return fmt.Errorf("print JSON output: %w", err)
	}

	return nil
}

// renderCertTable writes a human-readable summary of the chain, leaf first.
func renderCertTable(out io.Writer, info *certinfo.Info) error {
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)

	rows := [][2]string{
		{"Host:", info.Host},
		{"Address:", info.Addr},
		{"TLS version:", info.TLSVersion},
		{"Cipher suite:", info.CipherSuite},
		{"Server name (SNI):", info.ServerName},
		{"Chain depth:", strconv.Itoa(len(info.Chain))},
	}

	for _, row := range rows {
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", row[0], row[1]); err != nil {
			return fmt.Errorf("write table: %w", err)
		}
	}

	for i, cert := range info.Chain {
		label := "Certificate"
		if i > 0 {
			label = fmt.Sprintf("Certificate [%d]", i)
		}

		if _, err := fmt.Fprintf(tw, "%s:\t\n", label); err != nil {
			return fmt.Errorf("write table: %w", err)
		}

		certRows := certTableRows(i, cert)
		for _, row := range certRows {
			if _, err := fmt.Fprintf(tw, "  %s\t%s\n", row[0], row[1]); err != nil {
				return fmt.Errorf("write table: %w", err)
			}
		}
	}

	return tw.Flush()
}

// certTableRows builds the per-certificate rows, including the expiry
// warning computed against time.Now.
func certTableRows(index int, cert certinfo.Certificate) [][2]string {
	rows := [][2]string{
		{"Subject:", cert.Subject},
		{"Issuer:", cert.Issuer},
		{"Serial:", cert.SerialNumber},
		{"Not before:", cert.NotBefore.Format(time.RFC3339)},
		{"Not after:", cert.NotAfter.Format(time.RFC3339)},
	}

	if warn := expiryWarning(time.Now(), cert.NotAfter); warn != "" {
		rows = append(rows, [2]string{"Expiry:", warn})
	}

	if len(cert.DNSNames) > 0 {
		rows = append(rows, [2]string{"DNS names:", strings.Join(cert.DNSNames, ", ")})
	}

	if len(cert.IPAddresses) > 0 {
		rows = append(rows, [2]string{"IP addresses:", strings.Join(cert.IPAddresses, ", ")})
	}

	rows = append(rows,
		[2]string{"Signature algorithm:", cert.SignatureAlgo},
		[2]string{"Public key:", fmt.Sprintf("%s (%d bits)", cert.PublicKeyAlgo, cert.PublicKeyBits)},
		[2]string{"SHA-256 fingerprint:", cert.SHA256Fingerprint},
	)

	if index > 0 {
		caState := "no"
		if cert.IsCA {
			caState = "yes"
		}

		rows = append(rows, [2]string{"CA:", caState})
	}

	return rows
}

// expiryWarning returns a human-readable warning for a certificate that is
// expired or expiring within certExpiryWarnDays, or "" when fine.
func expiryWarning(now, notAfter time.Time) string {
	remaining := notAfter.Sub(now)

	switch {
	case remaining < 0:
		return fmt.Sprintf("EXPIRED %d days ago", int(-remaining.Hours()/24))
	case remaining <= certExpiryWarnDays*24*time.Hour:
		return fmt.Sprintf("WARNING: expires in %d days", int(remaining.Hours()/24)+1)
	default:
		return ""
	}
}

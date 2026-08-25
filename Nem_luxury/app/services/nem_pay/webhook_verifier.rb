require "openssl"

module NemPay
  # Verifies a NemPay webhook signature. NemPay signs the RAW delivered body with the shared
  # secret: X-NemPay-Signature = "sha256=" + hex(HMAC-SHA256(secret, body)). We recompute over the
  # exact bytes received and compare in constant time — verify BEFORE trusting/parsing the payload.
  module WebhookVerifier
    module_function

    def verify(raw_body, signature_header, secret)
      return false if signature_header.to_s.empty? || secret.to_s.empty?

      expected = "sha256=" + OpenSSL::HMAC.hexdigest("SHA256", secret, raw_body.to_s)
      ActiveSupport::SecurityUtils.secure_compare(signature_header.to_s, expected)
    end
  end
end

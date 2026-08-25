# NemPay integration configuration.
#
# NemLuxury talks to the NemPay gateway over HTTP only. All three values come from the
# environment (or Rails credentials) — the secret key and webhook secret must NEVER be
# committed or sent to the browser. Only `api_url` has a local-dev default.
#
#   NEMPAY_API_URL        base URL of the gateway (e.g. http://localhost:8080)
#   NEMPAY_SECRET_KEY     secret API key (sk_...) for server-to-server money calls
#   NEMPAY_WEBHOOK_SECRET shared HMAC secret used to verify inbound webhooks
#
# NEMPAY_WEBHOOK_SECRET must equal the secret NemPay is seeded with for this merchant's
# webhook endpoint (see plan task-05).
module NemPay
  module_function

  def api_url
    ENV.fetch("NEMPAY_API_URL", "http://localhost:8080")
  end

  # No defaults for the secrets — they must be provided (ENV / credentials), never baked into
  # the repo. Fetched lazily so boot and non-gateway commands don't require them; a gateway call
  # or webhook verification without them raises a clear error.
  def secret_key
    ENV.fetch("NEMPAY_SECRET_KEY")
  end

  def webhook_secret
    ENV.fetch("NEMPAY_WEBHOOK_SECRET")
  end
end

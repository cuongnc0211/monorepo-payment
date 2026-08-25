require "openssl"

# Helpers for delivering a (correctly or incorrectly) signed NemPay webhook to the app.
module NemPayWebhookHelpers
  def sign_webhook(body, secret: ENV.fetch("NEMPAY_WEBHOOK_SECRET"))
    "sha256=" + OpenSSL::HMAC.hexdigest("SHA256", secret, body)
  end

  def deliver_webhook(body:, event_type:, event_id: SecureRandom.uuid, signature: nil)
    post "/webhooks/nem_pay", params: body, headers: {
      "CONTENT_TYPE" => "application/json",
      "X-NemPay-Signature" => signature || sign_webhook(body),
      "X-NemPay-Event-Id" => event_id,
      "X-NemPay-Event-Type" => event_type
    }
  end
end

RSpec.configure do |config|
  config.include NemPayWebhookHelpers, type: :request
end

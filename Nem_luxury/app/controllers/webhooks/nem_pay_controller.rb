module Webhooks
  # Inbound NemPay webhooks. Handled SYNCHRONOUSLY (no background job): verify → dedup → update →
  # 200. NemPay's delivery worker is given a generous timeout to accommodate this.
  class NemPayController < ApplicationController
    # Webhooks are server-to-server and authenticated by the HMAC signature, not a CSRF token.
    skip_forgery_protection

    def create
      raw = request.raw_post

      # Verify BEFORE parsing or trusting anything. A bad/absent signature changes nothing.
      unless NemPay::WebhookVerifier.verify(raw, request.headers["X-NemPay-Signature"], NemPay.webhook_secret)
        return head :bad_request
      end

      event_id = request.headers["X-NemPay-Event-Id"].to_s
      event_type = request.headers["X-NemPay-Event-Type"].to_s
      return head :bad_request if event_id.empty? || event_type.empty?

      payload = JSON.parse(raw)
      return head :bad_request unless payload.is_a?(Hash)

      result = NemPay::WebhookHandler.new.call(event_id: event_id, event_type: event_type, payload: payload)
      Rails.logger.info("nem_pay webhook #{event_type} #{event_id} → #{result}")

      head :ok # 200 for processed / duplicate / unknown-intent alike, so NemPay stops retrying
    rescue JSON::ParserError
      head :bad_request
    end
  end
end

module NemPay
  # Orchestrates the direct-capture flow for one order: create → confirm(token) → capture.
  #
  # Each step uses a deterministic idempotency key derived from the order's checkout_token, so a
  # retry (or a double-submit) replays the same gateway operations instead of charging twice. It
  # NEVER marks the order paid — that is caused solely by the verified `captured` webhook. On a
  # decline it stops before capture and leaves the order pending_payment.
  class Checkout
    def initialize(client: Client.new)
      @client = client
    end

    # Returns a NemPay::Result: :ok (captured at the gateway), :declined, or :error.
    def call(order:, token:)
      base = "co-#{order.checkout_token}"

      create = @client.create_intent(
        amount_cents: order.amount_cents, currency: order.currency,
        metadata: { order_id: order.id.to_s }, idempotency_key: "#{base}-create"
      )
      return create unless create.ok?
      # A 2xx create must carry an intent id; a blank one would poison the confirm/capture URLs.
      return Result.error(error: "gateway returned no intent id") if create.intent_id.to_s.empty?

      order.update!(nem_pay_intent_id: create.intent_id)

      confirm = @client.confirm(intent_id: create.intent_id, token: token, idempotency_key: "#{base}-confirm")
      return confirm unless confirm.ok?
      if confirm.status == "failed"
        return Result.declined(intent_id: create.intent_id, status: confirm.status)
      end

      capture = @client.capture(intent_id: create.intent_id, idempotency_key: "#{base}-capture")
      return capture unless capture.ok?

      # Captured at the gateway — but the order stays pending_payment until the webhook confirms it.
      Result.ok(intent_id: create.intent_id, status: capture.status)
    end
  end
end

module NemPay
  # Orchestrates the direct-capture flow for one order. Split into two steps so a Stripe-style
  # checkout can create the PaymentIntent up front (contact step) and confirm+capture later (after
  # the card is tokenized), while `call` still runs the whole thing in one go for tests/back-compat.
  #
  # Each step uses a deterministic idempotency key derived from the order's checkout_token, so a
  # retry (or a double-submit) replays the same gateway operations instead of charging twice. It
  # NEVER marks the order paid — that is caused solely by the verified `captured` webhook.
  class Checkout
    def initialize(client: Client.new)
      @client = client
    end

    # Whole flow in one call: create → confirm(token) → capture. Returns a NemPay::Result.
    def call(order:, token:)
      created = create_intent(order)
      return created unless created.ok?

      pay(order: order, token: token)
    end

    # Step 1 — create the PaymentIntent and persist its id on the order. Idempotent per checkout.
    def create_intent(order)
      create = @client.create_intent(
        amount_cents: order.amount_cents, currency: order.currency,
        metadata: { order_id: order.id.to_s }, idempotency_key: "#{base(order)}-create"
      )
      return create unless create.ok?
      # A 2xx create must carry an intent id; a blank one would poison the confirm/capture URLs.
      return Result.error(error: "gateway returned no intent id") if create.intent_id.to_s.empty?

      order.update!(nem_pay_intent_id: create.intent_id)
      create
    end

    # Step 2 — confirm with the tokenized payment method, then capture. Requires an intent from
    # step 1. On a decline it stops before capture and returns a declined Result.
    def pay(order:, token:)
      intent_id = order.nem_pay_intent_id
      return Result.error(error: "order has no payment intent") if intent_id.to_s.empty?

      confirm = @client.confirm(intent_id: intent_id, token: token, idempotency_key: "#{base(order)}-confirm")
      return confirm unless confirm.ok?
      if confirm.status == "failed"
        return Result.declined(intent_id: intent_id, status: confirm.status)
      end

      capture = @client.capture(intent_id: intent_id, idempotency_key: "#{base(order)}-capture")
      return capture unless capture.ok?

      # Captured at the gateway — but the order stays pending_payment until the webhook confirms it.
      Result.ok(intent_id: intent_id, status: capture.status)
    end

    private

    def base(order)
      "co-#{order.checkout_token}"
    end
  end
end

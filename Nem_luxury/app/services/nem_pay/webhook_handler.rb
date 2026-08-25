module NemPay
  # Applies a verified webhook event to the order, exactly once.
  #
  # Dedup + side effect happen in ONE transaction: insert the ProcessedWebhookEvent (UNIQUE
  # event_id) and, for a captured payment, mark the order paid. A replayed event fails the insert
  # (RecordNotUnique) and is reported as a duplicate — a no-op the controller acknowledges with 200.
  # `paid` is set only here, and only from a verified captured payment.
  class WebhookHandler
    # Returns :processed, :duplicate, or :unknown_intent.
    def call(event_id:, event_type:, payload:)
      # NOTE (cross-component): event_id/event_type currently come from unsigned NemPay headers, so
      # the dedup KEY is not itself covered by the HMAC. Dedup integrity therefore rides on the side
      # effect (mark_paid!) being idempotent — which it is in M2. A future NemPay improvement should
      # put event_id (and type) into the SIGNED body so dedup is crypto-enforced, not just idempotent.
      intent_id = payload["id"]
      order = Order.find_by(nem_pay_intent_id: intent_id)

      ActiveRecord::Base.transaction do
        ProcessedWebhookEvent.create!(event_id: event_id, event_type: event_type, order_id: order&.id)
        apply(payload, order)
      end

      captured?(payload) && order.nil? ? :unknown_intent : :processed
    rescue ActiveRecord::RecordNotUnique
      :duplicate
    end

    private

    def apply(payload, order)
      # Decide off the SIGNED payload, never the unsigned X-NemPay-Event-Type header: the HMAC
      # covers the body only, so the "is this a capture?" decision must come from signed data
      # (`status`), or an attacker could relabel an authorized event as captured.
      return unless captured?(payload)
      return if order.nil?

      unless amounts_match?(payload, order)
        Rails.logger.warn("nem_pay webhook: captured amount/currency mismatch for order #{order.id}; not marking paid")
        return
      end

      order.mark_paid! # guarded + idempotent
    rescue Order::IllegalTransition => e
      # e.g. a captured event for a cancelled order: record it (dedup) and acknowledge — do NOT let
      # it raise, or NemPay would treat the 5xx as retryable and redeliver forever.
      Rails.logger.warn("nem_pay webhook: #{e.message} (order #{order&.id})")
    end

    # A captured payment, determined from signed fields only.
    def captured?(payload)
      payload.is_a?(Hash) && payload["object"] == "payment_intent" && payload["status"] == "captured"
    end

    def amounts_match?(payload, order)
      payload["amount"] == order.amount_cents && payload["currency"] == order.currency
    end
  end
end

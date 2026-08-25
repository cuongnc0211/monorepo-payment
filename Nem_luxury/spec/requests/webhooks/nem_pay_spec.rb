require 'rails_helper'

RSpec.describe "Webhooks::NemPay", type: :request do
  let(:secret) { ENV.fetch("NEMPAY_WEBHOOK_SECRET") }
  let(:product) { Product.create!(name: "Yacht", amount_cents: 14_900_000_00, currency: "USD") }
  let(:order) do
    Order.create!(product:, amount_cents: product.amount_cents, currency: "USD",
                  checkout_token: SecureRandom.uuid, nem_pay_intent_id: "pi_hook_1")
  end

  def payload_for(intent_id: order.nem_pay_intent_id, status: "captured")
    JSON.generate(id: intent_id, object: "payment_intent", status:, amount: order.amount_cents, currency: "USD")
  end

  def sign(body) = "sha256=" + OpenSSL::HMAC.hexdigest("SHA256", secret, body)

  def deliver(body, event_type: "payment_intent.captured", event_id: SecureRandom.uuid, signature: nil)
    post "/webhooks/nem_pay", params: body, headers: {
      "CONTENT_TYPE" => "application/json",
      "X-NemPay-Signature" => signature || sign(body),
      "X-NemPay-Event-Id" => event_id,
      "X-NemPay-Event-Type" => event_type
    }
  end

  it "marks the order paid on a verified captured event (AC1, AC3)" do
    body = payload_for
    expect { deliver(body) }.to change { order.reload.status }.from("pending_payment").to("paid")
    expect(response).to have_http_status(:ok)
    expect(ProcessedWebhookEvent.count).to eq(1)
  end

  it "dedupes a duplicate event — transitions once (AC4)" do
    body = payload_for
    id = SecureRandom.uuid
    deliver(body, event_id: id)
    deliver(body, event_id: id) # redelivery, same event_id

    expect(response).to have_http_status(:ok)
    expect(order.reload).to be_paid
    expect(ProcessedWebhookEvent.count).to eq(1)
  end

  it "rejects a bad signature and changes nothing (AC5)" do
    body = payload_for
    deliver(body, signature: "sha256=deadbeef")

    expect(response).to have_http_status(:bad_request)
    expect(order.reload).to be_pending_payment
    expect(ProcessedWebhookEvent.count).to eq(0)
  end

  it "rejects a body whose signature was computed with the wrong secret (AC5)" do
    body = payload_for
    wrong = "sha256=" + OpenSSL::HMAC.hexdigest("SHA256", "not-the-secret", body)
    deliver(body, signature: wrong)
    expect(response).to have_http_status(:bad_request)
    expect(order.reload).to be_pending_payment
  end

  it "acknowledges a non-captured event without changing the order (AC3)" do
    deliver(payload_for(status: "authorized"), event_type: "payment_intent.authorized")
    expect(response).to have_http_status(:ok)
    expect(order.reload).to be_pending_payment
  end

  it "acknowledges a captured event for an unknown intent without crashing" do
    order # ensure a real order exists but target a different intent
    deliver(payload_for(intent_id: "pi_unknown"))
    expect(response).to have_http_status(:ok)
    expect(order.reload).to be_pending_payment
  end

  it "rejects a delivery missing the event id" do
    deliver(payload_for, event_id: "")
    expect(response).to have_http_status(:bad_request)
  end

  # The trust-boundary case: the signature covers the BODY only. A validly-signed 'authorized'
  # body with the header relabelled 'captured' must NOT mark the order paid (AC3). The decision
  # keys off the signed payload status, not the header.
  it "ignores a captured header when the signed body says authorized (AC3)" do
    body = payload_for(status: "authorized") # correctly signed
    deliver(body, event_type: "payment_intent.captured") # attacker-relabelled header
    expect(response).to have_http_status(:ok)
    expect(order.reload).to be_pending_payment
  end

  it "does not mark paid when the captured amount/currency disagree with the order" do
    body = JSON.generate(id: order.nem_pay_intent_id, object: "payment_intent",
                         status: "captured", amount: 1, currency: "USD")
    deliver(body)
    expect(response).to have_http_status(:ok)
    expect(order.reload).to be_pending_payment
  end

  # A captured event for an order that isn't pending (e.g. cancelled) must be acknowledged, not
  # 5xx-looped: it is recorded (dedup) and ignored.
  it "acknowledges a captured event for a non-pending order without raising" do
    order.update!(status: :cancelled)
    deliver(payload_for)
    expect(response).to have_http_status(:ok)
    expect(order.reload).to be_cancelled
    expect(ProcessedWebhookEvent.count).to eq(1)
  end
end

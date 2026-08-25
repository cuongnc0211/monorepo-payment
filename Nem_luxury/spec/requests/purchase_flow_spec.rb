require 'rails_helper'

# Ties both halves together deterministically (NemPay stubbed): checkout drives the payment but
# leaves the order pending; the order becomes paid ONLY when the verified captured webhook arrives.
RSpec.describe "Purchase flow", type: :request do
  let!(:product) { Product.create!(name: "Aurora GT", amount_cents: 2_500_000_00, currency: "USD") }
  let(:base) { "http://localhost:8080" }
  let(:intent_id) { "pi_flow_1" }
  let(:token) { SecureRandom.uuid }

  before do
    stub_request(:post, "#{base}/v1/payment_intents")
      .to_return(status: 200, body: { id: intent_id, status: "created" }.to_json,
                 headers: { "Content-Type" => "application/json" })
    stub_request(:post, "#{base}/v1/payment_intents/#{intent_id}/confirm")
      .to_return(status: 200, body: { id: intent_id, status: "authorized" }.to_json,
                 headers: { "Content-Type" => "application/json" })
    stub_request(:post, "#{base}/v1/payment_intents/#{intent_id}/capture")
      .to_return(status: 200, body: { id: intent_id, status: "captured" }.to_json,
                 headers: { "Content-Type" => "application/json" })
  end

  def buy
    post checkout_path, params: { product_id: product.id, checkout_token: token, payment_method: "tok_ok" }
  end

  def captured_body
    JSON.generate(id: intent_id, object: "payment_intent", status: "captured",
                  amount: product.amount_cents, currency: "USD")
  end

  it "checkout leaves the order pending; the captured webhook then marks it paid (AC1, AC3)" do
    buy
    order = Order.find_by!(checkout_token: token)
    expect(order).to be_pending_payment # not paid from the synchronous capture response

    expect { deliver_webhook(body: captured_body, event_type: "payment_intent.captured") }
      .to change { order.reload.status }.from("pending_payment").to("paid")
  end

  it "without the webhook, the order stays pending (AC3 — never paid on redirect alone)" do
    buy
    order = Order.find_by!(checkout_token: token)
    expect(order).to be_pending_payment
    # No webhook delivered → still pending.
    expect(order.reload).to be_pending_payment
  end
end

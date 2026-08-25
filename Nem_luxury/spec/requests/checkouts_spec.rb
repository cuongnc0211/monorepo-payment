require 'rails_helper'

# Two-step checkout (spec 004): POST /checkout creates the order + a PaymentIntent (contact step),
# then POST /orders/:id/payment confirms+captures with a tokenized card (payment step). The order
# is marked paid only by the verified webhook, never by these synchronous responses.
RSpec.describe "Checkout", type: :request do
  let!(:product) { Product.create!(name: "Aurora GT", amount_cents: 2_500_000_00, currency: "USD") }
  let(:base) { "http://localhost:8080" }
  let(:token) { "co-token-#{SecureRandom.hex(4)}" }
  let(:intent_id) { "pi_test_1" }
  let(:contact) { { customer_name: "Ada", customer_address: "1 Maison Ave", customer_phone: "+377 1" } }

  def stub_create(status: "created")
    stub_request(:post, "#{base}/v1/payment_intents")
      .to_return(status: 200, body: { id: intent_id, status: status }.to_json,
                 headers: { "Content-Type" => "application/json" })
  end

  def stub_confirm(status:)
    stub_request(:post, "#{base}/v1/payment_intents/#{intent_id}/confirm")
      .to_return(status: 200, body: { id: intent_id, status: status }.to_json,
                 headers: { "Content-Type" => "application/json" })
  end

  def stub_capture
    stub_request(:post, "#{base}/v1/payment_intents/#{intent_id}/capture")
      .to_return(status: 200, body: { id: intent_id, status: "captured" }.to_json,
                 headers: { "Content-Type" => "application/json" })
  end

  # Step 1 — contact form submit.
  def start_checkout(tok: token)
    post checkout_path, params: { product_id: product.id, checkout_token: tok, **contact }
  end

  # Step 2 — pay with a tokenized card (the browser would have obtained `card_token` from NemPay).
  def pay(order, card_token:)
    post pay_order_path(order), params: { token: card_token }
  end

  describe "step 1 — create order + intent" do
    before { stub_create }

    it "stores the contact details and intent id, stays pending, and goes to the payment page" do
      start_checkout
      order = Order.find_by!(checkout_token: token)

      expect(order.customer_name).to eq("Ada")
      expect(order.nem_pay_intent_id).to eq(intent_id)
      expect(order).to be_pending_payment
      expect(response).to redirect_to(order_payment_path(order))
    end

    it "creates a DIRECT intent (no escrow/payee, no card data) with a per-op idempotency key" do
      start_checkout
      expect(a_request(:post, "#{base}/v1/payment_intents")
        .with(headers: { "Authorization" => "Bearer sk_test_nempay_secret",
                         "Idempotency-Key" => "co-#{token}-create" }) { |req|
          body = JSON.parse(req.body)
          body["amount"] == product.amount_cents && body["currency"] == "USD" &&
            body.key?("metadata") && !body.key?("escrow") && !body.key?("payee") &&
            !req.body.match?(/card|pan/i)
        }).to have_been_made
    end

    it "re-renders with an error and does not create an order when contact details are missing" do
      post checkout_path, params: { product_id: product.id, checkout_token: token }
      expect(response).to have_http_status(:unprocessable_entity)
      expect(Order.where(checkout_token: token)).to be_empty
    end
  end

  describe "step 2 — happy path" do
    before { stub_create; stub_confirm(status: "authorized"); stub_capture }

    it "confirms→captures with distinct idempotency keys but leaves the order pending" do
      start_checkout
      order = Order.find_by!(checkout_token: token)

      pay(order, card_token: "tok_ok")

      expect(order.reload).to be_pending_payment # paid is the webhook's job
      expect(response).to redirect_to(order_path(order))
      expect(a_request(:post, "#{base}/v1/payment_intents/#{intent_id}/confirm")
        .with(headers: { "Idempotency-Key" => "co-#{token}-confirm" })).to have_been_made
      expect(a_request(:post, "#{base}/v1/payment_intents/#{intent_id}/capture")
        .with(headers: { "Idempotency-Key" => "co-#{token}-capture" })).to have_been_made
    end
  end

  describe "step 2 — declined card" do
    before { stub_create; stub_confirm(status: "failed") }

    it "does not capture, keeps the order pending, and returns to the card page with a message" do
      start_checkout
      order = Order.find_by!(checkout_token: token)

      pay(order, card_token: "tok_declined")

      expect(order.reload).to be_pending_payment
      expect(a_request(:post, "#{base}/v1/payment_intents/#{intent_id}/capture")).not_to have_been_made
      expect(response).to redirect_to(order_payment_path(order))
      expect(flash[:alert]).to be_present
    end
  end

  describe "double-submit of step 1 (idempotent)" do
    before { stub_create }

    it "maps two submits of the same checkout_token to one order and one create call" do
      start_checkout
      start_checkout # resubmit, same token

      expect(Order.where(checkout_token: token).count).to eq(1)
      expect(a_request(:post, "#{base}/v1/payment_intents")).to have_been_made.once
    end
  end
end

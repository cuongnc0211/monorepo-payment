require 'rails_helper'

RSpec.describe "Checkout", type: :request do
  let!(:product) { Product.create!(name: "Aurora GT", amount_cents: 2_500_000_00, currency: "USD") }
  let(:base) { "http://localhost:8080" }
  let(:token) { "co-token-#{SecureRandom.hex(4)}" }
  let(:intent_id) { "pi_test_1" }

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

  def checkout(method:, tok: token)
    post checkout_path, params: { product_id: product.id, checkout_token: tok, payment_method: method }
  end

  describe "happy path (AC1, AC3)" do
    before do
      stub_create
      stub_confirm(status: "authorized")
      stub_capture
    end

    it "drives create→confirm→capture and stores the intent id, but does NOT mark the order paid" do
      checkout(method: "tok_ok")

      order = Order.find_by!(checkout_token: token)
      expect(order.nem_pay_intent_id).to eq(intent_id)
      expect(order).to be_pending_payment            # paid is the webhook's job, not the API response
      expect(response).to redirect_to(order_path(order))
    end

    it "authenticates with the secret key and a distinct Idempotency-Key per operation" do
      checkout(method: "tok_ok")
      expect(a_request(:post, "#{base}/v1/payment_intents")
        .with(headers: { "Authorization" => "Bearer sk_test_nempay_secret",
                         "Idempotency-Key" => "co-#{token}-create" })).to have_been_made
      expect(a_request(:post, "#{base}/v1/payment_intents/#{intent_id}/capture")
        .with(headers: { "Idempotency-Key" => "co-#{token}-capture" })).to have_been_made
    end

    it "creates a DIRECT intent — no escrow/payee, only amount+currency+metadata (AC8, AC7)" do
      checkout(method: "tok_ok")
      expect(a_request(:post, "#{base}/v1/payment_intents").with { |req|
        body = JSON.parse(req.body)
        body["amount"] == product.amount_cents && body["currency"] == "USD" &&
          body.key?("metadata") && !body.key?("escrow") && !body.key?("payee") &&
          !req.body.match?(/card|pan/i)
      }).to have_been_made
    end
  end

  describe "declined card (AC2)" do
    before do
      stub_create
      stub_confirm(status: "failed")
    end

    it "does not capture, leaves the order pending_payment, and shows a failure" do
      checkout(method: "tok_declined")

      order = Order.find_by!(checkout_token: token)
      expect(order).to be_pending_payment
      expect(a_request(:post, "#{base}/v1/payment_intents/#{intent_id}/capture")).not_to have_been_made
      expect(flash[:alert]).to be_present
    end
  end

  describe "double-submit (AC6)" do
    before do
      stub_create
      stub_confirm(status: "authorized")
      stub_capture
    end

    it "maps two submits of the same checkout_token to one order and one charge" do
      checkout(method: "tok_ok")
      checkout(method: "tok_ok") # resubmit, same token

      expect(Order.where(checkout_token: token).count).to eq(1)
      expect(a_request(:post, "#{base}/v1/payment_intents")).to have_been_made.once
    end
  end
end

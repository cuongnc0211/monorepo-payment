require 'rails_helper'

# Live end-to-end against a REAL NemPay + bank-sim. Skipped by default (tagged :e2e).
#
# Run it with the gateway up:
#   (in NemPay/)  docker-compose up
#   (here)        NEMPAY_API_URL=http://localhost:8080 bundle exec rspec --tag e2e
#
# This drives the outbound half (checkout → the real gateway, through to capture). The inbound
# webhook half (NemPay → this app → order paid) requires a running web server and is verified by
# the deterministic webhook specs plus the "Manual end-to-end" procedure in the README.
RSpec.describe "Live checkout against NemPay", :e2e, type: :request do
  it "drives an order to captured at the real gateway with a valid test card (AC1)" do
    product = Product.first || Product.create!(name: "E2E Item", amount_cents: 100_00, currency: "USD")
    order = Order.create!(product:, amount_cents: product.amount_cents, currency: product.currency,
                          checkout_token: SecureRandom.uuid)

    result = NemPay::Checkout.new.call(order:, token: "tok_ok")
    expect(result).to be_ok

    intent = NemPay::Client.new.get_intent(order.reload.nem_pay_intent_id)
    expect(%w[captured settled]).to include(intent.status)
  end

  it "leaves the order un-captured on a declined card (AC2)" do
    product = Product.first || Product.create!(name: "E2E Item", amount_cents: 100_00, currency: "USD")
    order = Order.create!(product:, amount_cents: product.amount_cents, currency: product.currency,
                          checkout_token: SecureRandom.uuid)

    result = NemPay::Checkout.new.call(order:, token: "tok_declined")
    expect(result).to be_declined
    expect(order.reload).to be_pending_payment
  end
end

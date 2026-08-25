require 'rails_helper'

RSpec.describe Order do
  let(:product) { Product.create!(name: "Test Yacht", amount_cents: 14_900_000_00, currency: "USD") }
  let(:order) do
    Order.create!(product:, amount_cents: product.amount_cents, currency: "USD",
                  checkout_token: SecureRandom.uuid)
  end

  describe "#mark_paid!" do
    it "moves a pending_payment order to paid" do
      expect { order.mark_paid! }.to change(order, :status).from("pending_payment").to("paid")
    end

    it "is idempotent when already paid" do
      order.mark_paid!
      expect { order.mark_paid! }.not_to change(order, :status)
    end

    it "refuses to mark a cancelled order as paid" do
      order.update!(status: :cancelled)
      expect { order.mark_paid! }.to raise_error(Order::IllegalTransition)
    end
  end

  it "stores money as an integer amount_cents" do
    expect(order.amount_cents).to be_an(Integer)
  end

  it "enforces checkout_token uniqueness" do
    dup = Order.new(product:, amount_cents: 1, currency: "USD", checkout_token: order.checkout_token)
    expect(dup).not_to be_valid
  end
end

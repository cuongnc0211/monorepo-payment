require 'rails_helper'

RSpec.describe "Products", type: :request do
  let!(:product) { Product.create!(name: "Aurora GT", description: "V12", amount_cents: 2_500_000_00, currency: "USD") }

  it "renders the catalogue with formatted prices" do
    get products_path
    expect(response).to have_http_status(:ok)
    expect(response.body).to include("Aurora GT")
    expect(response.body).to include("2,500,000.00 USD") # integer-cents formatting, no floats
  end

  describe "the product page (AC7 — PCI boundary)" do
    before { get product_path(product) }

    it "opens the multi-step checkout via an Acquire link, not an inline payment form" do
      expect(response).to have_http_status(:ok)
      expect(response.body).to include(new_checkout_path(product))
    end

    it "has no card-number input on the product page" do
      expect(response.body).not_to match(/card.?number/i)
      expect(response.body).not_to match(/name=["'](pan|card|cc_number|cardnumber)/i)
    end
  end
end

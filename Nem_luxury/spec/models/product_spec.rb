require 'rails_helper'

RSpec.describe Product do
  it "is valid with a name, positive integer price and a 3-letter currency" do
    expect(Product.new(name: "Watch", amount_cents: 180_000_00, currency: "USD")).to be_valid
  end

  it "rejects a non-positive price" do
    expect(Product.new(name: "Watch", amount_cents: 0, currency: "USD")).not_to be_valid
  end

  it "rejects a malformed currency" do
    expect(Product.new(name: "Watch", amount_cents: 1, currency: "US")).not_to be_valid
  end
end

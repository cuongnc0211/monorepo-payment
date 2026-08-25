# A small luxury catalogue. Prices are integer cents (USD).
[
  { name: "Aurora GT Supercar",       amount_cents: 2_500_000_00, currency: "USD",
    description: "Hand-built V12, 0–100 in 2.8s. One of twelve." },
  { name: "Meridian Tourbillon Watch", amount_cents:    180_000_00, currency: "USD",
    description: "Flying tourbillon, 18k rose gold, sapphire case back." },
  { name: "Halcyon 120 Yacht",         amount_cents: 14_900_000_00, currency: "USD",
    description: "36m superyacht, four staterooms, beach club." },
  { name: "Celeste Private Jet",       amount_cents: 48_000_000_00, currency: "USD",
    description: "Ultra-long-range jet, 14 passengers, transpacific." }
].each do |attrs|
  Product.find_or_create_by!(name: attrs[:name]) do |p|
    p.amount_cents = attrs[:amount_cents]
    p.currency     = attrs[:currency]
    p.description  = attrs[:description]
  end
end

puts "Seeded #{Product.count} products."

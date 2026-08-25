# A catalogue item. Prices are integer minor units (cents) + an ISO-4217 currency — never floats.
# The product owns its meaning (a yacht vs a watch); NemPay never sees it, only an amount.
class Product < ApplicationRecord
  has_many :orders, dependent: :restrict_with_error

  validates :name, presence: true
  validates :amount_cents, numericality: { only_integer: true, greater_than: 0 }
  validates :currency, presence: true, length: { is: 3 }
end

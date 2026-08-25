# An order for one product. Its lifecycle is:
#
#   pending_payment → paid → fulfilled | cancelled
#
# The `paid` transition is caused ONLY by a verified NemPay `payment_intent.captured` webhook
# (see Webhooks::NemPayController) — never by a browser redirect or a synchronous API response.
# `fulfilled` and `cancelled` exist for realism but have no workflow driving them in M2.
class Order < ApplicationRecord
  # Raised when a caller attempts a transition the lifecycle does not allow.
  class IllegalTransition < StandardError; end

  belongs_to :product

  enum :status, { pending_payment: 0, paid: 1, fulfilled: 2, cancelled: 3 }

  validates :amount_cents, numericality: { only_integer: true, greater_than: 0 }
  validates :currency, presence: true, length: { is: 3 }
  validates :checkout_token, presence: true, uniqueness: true

  # Contact details are required only when submitted through the checkout form (context :checkout),
  # so internal order creation and existing flows are unaffected.
  validates :customer_name, :customer_address, :customer_phone, presence: true, on: :checkout

  # Move the order to `paid`. Only legal from `pending_payment`; idempotent if already `paid`
  # (at-least-once webhook delivery means this may be called more than once). Any other source
  # state is a bug and raises.
  def mark_paid!
    return true if paid?
    unless pending_payment?
      raise IllegalTransition, "cannot mark a #{status} order as paid"
    end

    update!(status: :paid)
  end
end

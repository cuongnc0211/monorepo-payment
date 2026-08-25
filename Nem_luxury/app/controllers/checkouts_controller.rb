class CheckoutsController < ApplicationController
  # Buy now. The order is keyed by the form's checkout_token, so a double-submit of the same form
  # maps to ONE order (and one charge). The payment is driven only when the order is first created
  # in this request; a resubmit just returns the existing order's status.
  def create
    product = Product.find(params[:product_id])
    token = params[:checkout_token].to_s
    if token.blank?
      redirect_to product_path(product), alert: "Your checkout session expired — please try again." and return
    end

    order, created = find_or_create_order(product, token)

    # Drive the payment ONLY in the request that actually inserted the order row. This makes the
    # single-drive guarantee app-authoritative, not merely reliant on the gateway's idempotency:
    # a concurrent double-submit has exactly one inserter (the other loses the unique race below).
    #
    # Scope note (M2): a transient gateway error mid-flight leaves the order pending_payment; a
    # resubmit of the SAME token will not re-drive (it's no longer the inserter). Auto-retry of
    # partial failures is deliberately out of M2 scope — the customer starts a fresh checkout.
    if created
      result = NemPay::Checkout.new.call(order: order, token: params[:payment_method].to_s)
      flash[:alert] = checkout_alert(result) unless result.ok?
    end

    redirect_to order_path(order)
  end

  private

  # One order per checkout_token. Returns [order, created?] where created? is true ONLY for the
  # request that inserted the row. find_or_create_by! runs its block during `new` (before save),
  # so under a race both builders set created=true; the loser then hits the unique index, is
  # rescued to the winning row, and has its created reset to false — so it never drives payment.
  def find_or_create_order(product, token)
    created = false
    order = Order.find_or_create_by!(checkout_token: token) do |o|
      o.product = product
      o.amount_cents = product.amount_cents
      o.currency = product.currency
      created = true
    end
    [order, created]
  rescue ActiveRecord::RecordNotUnique
    [Order.find_by!(checkout_token: token), false]
  end

  def checkout_alert(result)
    if result.declined?
      "Your payment was declined. No charge was made — you can try again."
    else
      "We couldn’t reach the payment gateway. Please try again."
    end
  end
end

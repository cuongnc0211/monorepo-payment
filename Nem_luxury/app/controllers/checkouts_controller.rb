class CheckoutsController < ApplicationController
  # Step 1 (form): collect the customer's contact details. A fresh checkout_token is minted here so
  # a double-submit of THIS form maps to a single order (and a single charge) downstream.
  def new
    @product = Product.find(params[:product_id])
    @checkout_token = SecureRandom.uuid
    @order = Order.new
  end

  # Step 1 (submit): create the order with the contact details AND a NemPay PaymentIntent, then send
  # the customer to the card page. The intent is created here — mirroring Stripe, where the intent
  # exists before the card is entered — and confirmed+captured later on the payment page.
  def create
    product = Product.find(params[:product_id])
    token = params[:checkout_token].to_s
    if token.blank?
      redirect_to product_path(product), alert: "Your checkout session expired — please try again." and return
    end

    @product = product
    @checkout_token = token
    @order = build_order(product, token)

    unless @order.valid?(:checkout)
      return render :new, status: :unprocessable_entity
    end

    order, created = find_or_create_order(product, token)

    # Create the gateway intent only on the request that actually inserted the order row.
    if created && order.nem_pay_intent_id.blank?
      result = NemPay::Checkout.new.create_intent(order)
      unless result.ok?
        redirect_to new_checkout_path(product), alert: "We couldn’t start checkout with the payment gateway. Please try again." and return
      end
    end

    redirect_to order_payment_path(order)
  end

  private

  def order_params
    params.permit(:customer_name, :customer_address, :customer_phone)
  end

  # An unsaved order carrying the submitted contact details, used to validate before we touch the
  # gateway. Not persisted here — find_or_create_order does the insert once validation passes.
  def build_order(product, token)
    Order.new(
      product: product, amount_cents: product.amount_cents, currency: product.currency,
      checkout_token: token, **order_params.to_h.symbolize_keys
    )
  end

  # One order per checkout_token. Returns [order, created?] where created? is true ONLY for the
  # request that inserted the row (see the unique-constraint race handling in the rescue).
  def find_or_create_order(product, token)
    created = false
    order = Order.find_or_create_by!(checkout_token: token) do |o|
      o.product = product
      o.amount_cents = product.amount_cents
      o.currency = product.currency
      o.customer_name = order_params[:customer_name]
      o.customer_address = order_params[:customer_address]
      o.customer_phone = order_params[:customer_phone]
      created = true
    end
    [order, created]
  rescue ActiveRecord::RecordNotUnique
    [Order.find_by!(checkout_token: token), false]
  end
end

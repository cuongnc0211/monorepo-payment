class PaymentsController < ApplicationController
  before_action :load_order

  # Step 2 (card page): the Stripe-like card form. Card data is entered here but tokenized in the
  # browser directly against NemPay — this server only ever receives the resulting token.
  def new
    redirect_to order_path(@order) and return if @order.paid?
    # If the intent was never created (gateway hiccup at step 1), restart the checkout cleanly.
    redirect_to new_checkout_path(@order.product), alert: "Please start your order again." and return if @order.nem_pay_intent_id.blank?
  end

  # Step 2 (submit): confirm + capture using the tokenized payment method. The order is marked paid
  # only later, by the verified capture webhook — never here.
  def create
    redirect_to order_path(@order) and return if @order.paid?

    token = params[:token].to_s
    if token.blank?
      redirect_to order_payment_path(@order), alert: "Please enter your card details." and return
    end

    result = NemPay::Checkout.new.pay(order: @order, token: token)

    if result.ok?
      redirect_to order_path(@order)
    elsif result.respond_to?(:declined?) && result.declined?
      redirect_to order_payment_path(@order), alert: "Your card was declined. No charge was made — please try another card."
    else
      redirect_to order_payment_path(@order), alert: "We couldn’t reach the payment gateway. Please try again."
    end
  end

  private

  def load_order
    @order = Order.find(params[:id])
  end
end

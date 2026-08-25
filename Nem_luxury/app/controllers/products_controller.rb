class ProductsController < ApplicationController
  def index
    @products = Product.order(:amount_cents)
  end

  def show
    @product = Product.find(params[:id])
    # A fresh per-render checkout token: a double-submit of THIS buy form reuses this token, so
    # it maps to a single order (and a single charge) downstream — see CheckoutsController.
    @checkout_token = SecureRandom.uuid
  end
end

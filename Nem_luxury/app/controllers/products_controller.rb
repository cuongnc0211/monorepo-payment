class ProductsController < ApplicationController
  def index
    @products = Product.order(:amount_cents)
  end

  def show
    @product = Product.find(params[:id])
    # "Acquire now" now opens the multi-step checkout (contact → card); the checkout_token is minted
    # on the contact form (CheckoutsController#new), not here.
  end
end

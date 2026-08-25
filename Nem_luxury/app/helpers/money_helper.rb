module MoneyHelper
  # Format integer minor units for display, e.g. (250_000_00, "USD") → "250,000.00 USD".
  # Uses integer math only — no float ever enters the money path, display included.
  def format_money(cents, currency)
    dollars, remainder = cents.divmod(100)
    "#{number_with_delimiter(dollars)}.#{format('%02d', remainder)} #{currency}"
  end
end

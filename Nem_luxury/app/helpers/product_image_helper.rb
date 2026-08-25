module ProductImageHelper
  # Presentation-only mapping from a catalogue item to a stock photo.
  #
  # NemLuxury owns product *meaning*; which photograph represents it is a front-end concern, so
  # it lives in this helper rather than in the model or the DB. The primary source is Unsplash
  # (fixed, verified asset ids). If an Unsplash asset ever fails to load, the <img> onerror swaps
  # in a keyword-matched LoremFlickr photo, so a relevant image always renders — even offline of
  # any single asset. Also carries a category label used as the card eyebrow.
  IMAGES = [
    { match: /supercar|\bgt\b/i,   category: "Automotive", unsplash: "1503376780353-7e6692767b70", keyword: "supercar,luxury",   lock: 21 },
    { match: /watch|tourbillon/i,  category: "Horology",   unsplash: "1523275335684-37898b6baf30", keyword: "luxury,watch",       lock: 22 },
    { match: /yacht/i,             category: "Marine",     unsplash: "1567899378494-47b22a2ae96a", keyword: "superyacht,luxury",  lock: 23 },
    { match: /jet|aircraft/i,      category: "Aviation",   unsplash: "1540962351504-03099e0a754b", keyword: "private,jet",        lock: 24 }
  ].freeze

  DEFAULT = { category: "Collection", unsplash: "1503376780353-7e6692767b70", keyword: "luxury", lock: 20 }.freeze

  ASPECT = 1.25 # 4:5 portrait, matches the CSS media boxes

  def product_meta(product)
    IMAGES.find { |m| product.name.to_s.match?(m[:match]) } || DEFAULT
  end

  def product_category(product)
    product_meta(product)[:category]
  end

  # Render a product <img> with a graceful keyword fallback and correct aspect ratio (no CLS).
  def product_image_tag(product, width:, css_class: nil, eager: false)
    meta   = product_meta(product)
    height = (width * ASPECT).round
    src      = "https://images.unsplash.com/photo-#{meta[:unsplash]}?auto=format&fit=crop&w=#{width}&h=#{height}&q=80"
    fallback = "https://loremflickr.com/#{width}/#{height}/#{meta[:keyword]}?lock=#{meta[:lock]}"

    tag.img(
      src: src,
      alt: product.name,
      class: css_class,
      width: width,
      height: height,
      loading: eager ? "eager" : "lazy",
      decoding: "async",
      onerror: "this.onerror=null;this.src='#{fallback}'"
    )
  end
end

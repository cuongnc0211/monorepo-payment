module ApplicationHelper
  # Inline Lucide-style stroke icons. Vector only — never emoji — so they scale cleanly, inherit
  # `currentColor`, and stay consistent in stroke weight across the UI.
  ICONS = {
    "arrow-left"  => '<path d="M19 12H5M12 19l-7-7 7-7"/>',
    "arrow-right" => '<path d="M5 12h14M12 5l7 7-7 7"/>',
    "check"       => '<path d="M20 6 9 17l-5-5"/>',
    "shield"      => '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>',
    "gem"         => '<path d="M6 3h12l4 6-10 12L2 9z"/><path d="M2 9h20M12 3 8 9l4 12 4-12-4-6"/>',
    "spinner"     => '<circle cx="12" cy="12" r="9" opacity=".25"/><path d="M21 12a9 9 0 0 0-9-9"/>'
  }.freeze

  def icon(name, size: 18, spin: false)
    body = ICONS[name.to_s]
    return "".html_safe if body.nil?

    css = spin ? "icon icon--spin" : "icon"
    content_tag(:svg, body.html_safe,
                class: css, width: size, height: size, viewBox: "0 0 24 24",
                fill: "none", stroke: "currentColor", "stroke-width": "1.6",
                "stroke-linecap": "round", "stroke-linejoin": "round", "aria-hidden": "true")
  end
end

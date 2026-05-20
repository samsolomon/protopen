# Site Anchors

Protopen comments are pinned to a real DOM element, not to document
coordinates. This means a pin stays attached to the same component when
the viewport is resized, the layout reflows for mobile, or you re-deploy
with HTML changes that don't disturb the element.

When you click to add a comment, the runtime walks up from the click
target looking for the most stable identifier it can find and stores
that selector with the comment. At render time it resolves the selector
back to an element and places the pin using the offset captured at
click time, relative to the element's current bounding box.

## Capture priority

The runtime tries these in order and stops at the first one that works:

1. **`data-comment-anchor`** — your explicit anchor attribute. Highest
   priority. Walks up to 4 ancestors.
2. **`data-testid`, `data-id`, `data-component`** — common test/component
   identifiers, treated as opportunistic anchors. Same 4-ancestor walk.
3. **`#id`** — a stable id on the click target itself. Skipped when the
   id looks framework-generated (long hex strings, etc.).
4. **`nth-of-type` structural path** — up to 5 ancestor levels of
   `tag:nth-of-type(n)` joined with `>`. The fragile fallback.
5. **`body`** — terminal anchor. Always wins if nothing above did.

Steps 4 and 5 work without any cooperation from the site, but they're
brittle: a `nth-of-type` selector breaks when a sibling is inserted
above the anchored element, and `body` covers the whole page so the
offset-within-element math becomes equivalent to document coordinates
(which is the drift problem this system replaces).

## The `data-comment-anchor` convention

Add `data-comment-anchor="<short-stable-name>"` to elements you expect
to comment on:

```html
<section data-comment-anchor="hero">…</section>
<button data-comment-anchor="cta-primary">Get started</button>
<div data-comment-anchor="pricing-table">…</div>
```

Then a comment clicked anywhere inside `<section data-comment-anchor="hero">`
captures `[data-comment-anchor="hero"]` and lives there forever — even
if you refactor the surrounding HTML, swap your CSS framework, or move
the section between deploys. The offset stored is relative to the
matched element's box, so the pin lands at the same visual spot
regardless of how the element reflows.

## When to add anchors

- **Commentable regions you care about long-term.** Hero, primary CTAs,
  pricing tables, signup forms — anywhere comments will accumulate and
  you want them to survive HTML changes.
- **Repeated elements** (cards in a grid, list items). Without an
  explicit anchor the runtime falls back to `nth-of-type`, which loses
  comments when items are reordered or inserted.
- **Components that already use `data-testid`.** The runtime treats
  `data-testid` as a valid anchor, so you may already be covered.

## When to skip anchors

- **One-off prototypes** where comments don't need to outlive the next
  deploy. The structural fallback works fine for short-lived sites.
- **Components without a stable identity** (e.g. animated stages of a
  carousel). Anchoring to a stable parent + accepting drift is usually
  better than pretending each child is stable.

## What the runtime does at render time

- Resolves the stored selector via `document.querySelectorAll`. If
  exactly one element matches, the pin renders there.
- If the selector matches zero or many elements, the pin is **skipped**
  (not rendered). This is the trade-off for honesty: better than
  rendering at the document origin and pretending the anchor worked.
- If the resolved element is hidden (zero bounding box from
  `display:none`, an unmounted route, a collapsed accordion), the pin
  is also skipped until the element re-appears.

## Drag-to-reanchor

Dragging an existing pin doesn't move it in document coordinates.
Instead, on drop the runtime hit-tests the element under the cursor
with `elementFromPoint`, runs the same capture logic against it, and
PATCHes the comment with the new selector + offset. So dragging is the
authoritative way to re-anchor a pin that ended up on the wrong element.

## Storage shape

A comment row stores three fields for the anchor:

| Column            | Example                                  |
|-------------------|------------------------------------------|
| `element_selector`| `[data-comment-anchor="hero-cta"]`       |
| `element_offset_x`| `0.42` (0..1 within the element's width) |
| `element_offset_y`| `0.78` (0..1 within the element's height)|

Replies (`parent_id` set) carry no anchor — only root comments render
as pins.

## Cross-deploy behavior

Comments are site-scoped: a new deploy doesn't create new comments. The
runtime queries `/api/sites/{id}/comments?pagePath=…` without
`deployId`, so every open comment on that page is returned regardless
of which deploy first saw it. The anchor is what makes the comment
appear in the right place on the new deploy — if your anchor is stable
across the refactor, the comment "follows". If it isn't, the pin
silently doesn't render, and an org admin can drag any existing pin to
re-anchor it.

Deleting a deploy preserves the comments that were first seen on it —
the FK is `ON DELETE SET NULL`, not cascade.

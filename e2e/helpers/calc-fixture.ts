/**
 * The category header and group Meta shared by the [calc] E2E and a11y specs.
 */
export const CALC_HEADER = [
  '<div class="calc-test">',
  '<p>Total due: <span class="calc-total">[calc expr="meta.qty * meta.unit_price * (1 - coalesce(meta.discount, 0))" decimals="2" default="-"]</span></p>',
  '<p>Deposit: <span class="calc-missing">[calc expr="meta.deposit * 2" default="not set"]</span></p>',
  '<table class="calc-lines"><caption>Invoice lines</caption>',
  '<thead><tr><th scope="col">Line</th><th scope="col">Total</th></tr></thead>',
  '<tbody>[each path="lines"]<tr><td>[item index="true"]</td><td class="calc-line">[calc expr="item.qty * item.price" decimals="2"]</td></tr>[/each]</tbody>',
  '</table>',
  '<div class="calc-qty">Quantity: [meta path="qty" editable="true"]</div>',
  '[autorefresh]<p>Live total: <span class="calc-live">[calc expr="meta.qty * meta.unit_price" decimals="2"]</span></p>[/autorefresh]',
  '</div>',
].join('\n');

export const CALC_META = {
  qty: 3,
  unit_price: 12.5,
  discount: 0.1,
  lines: [
    { qty: 2, price: 1.25 },
    { qty: 1, price: 4 },
  ],
};

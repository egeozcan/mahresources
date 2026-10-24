// @vitest-environment happy-dom
import { afterEach, describe, expect, it } from 'vitest';
import { containFocus, tabStops } from './containFocus.js';

const ids = (root: Element) => tabStops(root).map((el) => el.id);

function press(shiftKey = false) {
  const event = new KeyboardEvent('keydown', { key: 'Tab', shiftKey, bubbles: true, cancelable: true });
  (document.activeElement || document.body).dispatchEvent(event);
  return event;
}

afterEach(() => { document.body.innerHTML = ''; });

describe('tab stops follow native traversal', () => {
  it('skips negative tabindex, disabled controls and disabled fieldsets', () => {
    document.body.innerHTML = `<div id="root">
      <button id="a">a</button>
      <button id="neg" tabindex="-1">neg</button>
      <button id="dis" disabled>dis</button>
      <fieldset disabled><button id="inFieldset">x</button><input id="inFieldsetInput"></fieldset>
      <fieldset disabled><legend><button id="inLegend">legend controls stay enabled</button></legend></fieldset>
      <input id="b">
    </div>`;
    expect(ids(document.getElementById('root')!)).toEqual(['a', 'inLegend', 'b']);
  });

  it('skips hidden, invisible and inert controls', () => {
    document.body.innerHTML = `<div id="root">
      <button id="a">a</button>
      <button id="vis" style="visibility:hidden">vis</button>
      <div style="display:none"><button id="none">none</button></div>
      <div inert><button id="inert">inert</button></div>
      <button id="b">b</button>
    </div>`;
    expect(ids(document.getElementById('root')!)).toEqual(['a', 'b']);
  });

  it('treats a radio group as one stop: the checked radio, else the first', () => {
    document.body.innerHTML = `<div id="root">
      <input type="radio" name="g1" id="g1a"><input type="radio" name="g1" id="g1b" checked><input type="radio" name="g1" id="g1c">
      <input type="radio" name="g2" id="g2a"><input type="radio" name="g2" id="g2b">
      <input type="radio" id="lone1"><input type="radio" id="lone2">
    </div>`;
    expect(ids(document.getElementById('root')!)).toEqual(['g1b', 'g2a', 'lone1', 'lone2']);
  });
});

describe('stacked containments', () => {
  it('a top dialog removed without stop() no longer disables the one below', () => {
    document.body.innerHTML = `<div id="lower"><button id="l1">1</button><button id="l2">2</button></div><div id="upper"><button id="u1">u</button></div>`;
    const lower = containFocus(document.getElementById('lower'));
    containFocus(document.getElementById('upper'));
    document.getElementById('upper')!.remove();
    document.getElementById('l2')!.focus();
    const event = press();
    expect(event.defaultPrevented).toBe(true);
    expect(document.activeElement?.id).toBe('l1');
    lower.stop();
  });
});

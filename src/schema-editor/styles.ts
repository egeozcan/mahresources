import { css } from 'lit';

export const sharedStyles = css`
  :host {
    display: block;
    font-family: system-ui, -apple-system, sans-serif;
    font-size: 13px;
    color: #1f2937;
  }

  * {
    box-sizing: border-box;
  }

  /* ─── Tree badges ─────────────────────────── */
  .badge {
    display: inline-block;
    padding: 0 6px;
    border-radius: 3px;
    font-size: 10px;
    font-weight: 600;
    line-height: 18px;
  }
  .badge-string { background: #d1fae5; color: #065f46; }
  .badge-number, .badge-integer { background: #dbeafe; color: #1e40af; }
  .badge-boolean { background: #fef9c3; color: #854d0e; }
  .badge-object { background: #e0e7ff; color: #3730a3; }
  .badge-array { background: #ede9fe; color: #5b21b6; }
  .badge-enum { background: #fef3c7; color: #92400e; }
  .badge-composition { background: #fce7f3; color: #9d174d; }
  .badge-ref { background: #f3f4f6; color: #6b7280; }
  .badge-def { background: #f1f5f9; color: #475569; }
  .badge-conditional { background: #fff1f2; color: #9f1239; }

  /* ─── Form elements ───────────────────────── */
  input, select, textarea {
    width: 100%;
    padding: 5px 8px;
    border: 1px solid #d1d5db;
    border-radius: 4px;
    font-size: 12px;
    font-family: inherit;
  }
  input:focus, select:focus, textarea:focus {
    outline: none;
    border-color: #6366f1;
    box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.2);
  }
  input[type="checkbox"] {
    width: auto;
    margin-right: 4px;
  }
  input[type="number"] {
    -moz-appearance: textfield;
  }
  label {
    display: block;
    font-size: 11px;
    color: #6b7280;
    margin-bottom: 2px;
    font-weight: 600;
  }

  /* ─── Buttons ─────────────────────────────── */
  button {
    cursor: pointer;
    font-family: inherit;
  }
  .btn {
    padding: 4px 12px;
    border: 1px solid #d1d5db;
    border-radius: 4px;
    background: white;
    font-size: 11px;
    color: #374151;
  }
  .btn:hover { background: #f9fafb; }
  .btn:focus-visible { outline: 2px solid #6366f1; outline-offset: 2px; }
  .btn-primary {
    background: #4338ca;
    color: white;
    border-color: #4338ca;
  }
  .btn-primary:hover { background: #3730a3; }
  .btn-danger {
    background: #fee2e2;
    color: #dc2626;
    border-color: #fecaca;
  }
  .btn-danger:hover { background: #fecaca; }
  .btn-ghost {
    background: transparent;
    border: 1px dashed #d1d5db;
    color: #6b7280;
    width: 100%;
    text-align: center;
  }

  /* ─── Utility ─────────────────────────────── */
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border-width: 0;
  }
  .required-marker {
    color: #dc2626;
    font-size: 9px;
    margin-left: 2px;
  }
  .breadcrumb {
    font-size: 11px;
    color: #9ca3af;
    margin-bottom: 8px;
  }
  .breadcrumb .current {
    color: #4338ca;
  }
`;

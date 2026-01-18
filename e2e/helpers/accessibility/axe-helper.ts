import { Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import type { Result, NodeResult } from 'axe-core';

/**
 * WCAG 2.1 Level AA tags for axe-core
 * These are the standards we test against
 */
export const WCAG_AA_TAGS = [
  'wcag2a',
  'wcag2aa',
  'wcag21a',
  'wcag21aa',
];

/**
 * Known accessibility issues that are tracked for later fixes
 * Add issues here to allow tests to pass while acknowledging the problem
 */
export const KNOWN_ISSUES: KnownIssue[] = [
  // No known issues - all accessibility violations should be fixed in the code
];

export interface KnownIssue {
  id: string;
  description: string;
  pages?: string[];
  ticket?: string;
}

export interface A11yCheckOptions {
  /**
   * Exclude selectors from accessibility checks
   */
  exclude?: string[];
  /**
   * Include only specific selectors
   */
  include?: string[];
  /**
   * Axe rule IDs to skip
   */
  disableRules?: string[];
  /**
   * Filter violations against known issues
   */
  filterKnownIssues?: boolean;
}

/**
 * Format a single violation for readable output
 */
function formatNode(node: NodeResult): string {
  const lines: string[] = [];
  lines.push(`    Target: ${node.target.join(', ')}`);
  if (node.html) {
    const html = node.html.length > 100 ? node.html.substring(0, 100) + '...' : node.html;
    lines.push(`    HTML: ${html}`);
  }
  if (node.failureSummary) {
    lines.push(`    Fix: ${node.failureSummary.split('\n')[0]}`);
  }
  return lines.join('\n');
}

/**
 * Format violations into a readable string for test output
 */
export function formatViolations(violations: Result[]): string {
  if (violations.length === 0) {
    return 'No accessibility violations found';
  }

  const lines: string[] = [
    `Found ${violations.length} accessibility violation(s):`,
    '',
  ];

  violations.forEach((violation, index) => {
    lines.push(`${index + 1}. ${violation.id}: ${violation.description}`);
    lines.push(`   Impact: ${violation.impact}`);
    lines.push(`   Help: ${violation.helpUrl}`);
    lines.push(`   Affected elements (${violation.nodes.length}):`);

    // Show first 3 affected nodes to keep output manageable
    violation.nodes.slice(0, 3).forEach(node => {
      lines.push(formatNode(node));
    });

    if (violation.nodes.length > 3) {
      lines.push(`    ... and ${violation.nodes.length - 3} more`);
    }
    lines.push('');
  });

  return lines.join('\n');
}

/**
 * Filter out known issues from violations
 */
function filterKnownIssues(violations: Result[], currentPage?: string): Result[] {
  return violations.filter(violation => {
    const knownIssue = KNOWN_ISSUES.find(issue => issue.id === violation.id);
    if (!knownIssue) return true;

    // If known issue is page-specific, only filter on those pages
    if (knownIssue.pages && currentPage) {
      return !knownIssue.pages.some(page => currentPage.includes(page));
    }

    // Global known issue - always filter
    return false;
  });
}

/**
 * Run accessibility check on the full page
 * Returns the axe results for further processing
 */
export async function checkAccessibility(
  page: Page,
  options: A11yCheckOptions = {}
): Promise<Result[]> {
  let builder = new AxeBuilder({ page })
    .withTags(WCAG_AA_TAGS);

  if (options.exclude) {
    options.exclude.forEach(selector => {
      builder = builder.exclude(selector);
    });
  }

  if (options.include) {
    options.include.forEach(selector => {
      builder = builder.include(selector);
    });
  }

  if (options.disableRules) {
    builder = builder.disableRules(options.disableRules);
  }

  const results = await builder.analyze();
  let violations = results.violations;

  if (options.filterKnownIssues) {
    violations = filterKnownIssues(violations, page.url());
  }

  return violations;
}

/**
 * Assert that the page has no accessibility violations
 * Throws an error with formatted output if violations are found
 */
export async function expectNoViolations(
  page: Page,
  options: A11yCheckOptions = {}
): Promise<void> {
  const violations = await checkAccessibility(page, {
    filterKnownIssues: true,
    ...options,
  });

  if (violations.length > 0) {
    throw new Error(formatViolations(violations));
  }
}

/**
 * Check accessibility of a specific component/element
 */
export async function checkComponentAccessibility(
  page: Page,
  selector: string,
  options: Omit<A11yCheckOptions, 'include'> = {}
): Promise<Result[]> {
  return checkAccessibility(page, {
    ...options,
    include: [selector],
  });
}

/**
 * Assert that a specific component has no accessibility violations
 */
export async function expectComponentNoViolations(
  page: Page,
  selector: string,
  options: Omit<A11yCheckOptions, 'include'> = {}
): Promise<void> {
  const violations = await checkComponentAccessibility(page, selector, {
    filterKnownIssues: true,
    ...options,
  });

  if (violations.length > 0) {
    throw new Error(
      `Component "${selector}" has accessibility violations:\n${formatViolations(violations)}`
    );
  }
}

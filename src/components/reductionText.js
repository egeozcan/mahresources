/** `1 copy`, `2 copies`: the count and the right form of the noun. */
export function plural(count, one, many) {
  return `${count} ${count === 1 ? one : many}`;
}

/** The plural form of the nouns this page counts. */
export const CLUSTER_NOUN = ['Cluster', 'Clusters'];
export const COPIES = ['copy', 'copies'];

/** YYYY-MM-DD in the reviewer's own time zone. */
export function today() {
  const now = new Date();
  const month = String(now.getMonth() + 1).padStart(2, '0');
  const day = String(now.getDate()).padStart(2, '0');
  return `${now.getFullYear()}-${month}-${day}`;
}

/**
 * A name that tells Reductions apart in the list. The server's own default stays
 * "Resource Reduction"; this is what the forms send when the reviewer left the
 * field empty.
 */
export function defaultReductionName({ all = false, groups = 0, resources = 0, groupName = '' } = {}) {
  const date = today();
  let what;
  if (all) what = 'everything';
  else if (groupName) what = groupName;
  else if (groups && resources) what = `${plural(groups, 'Group', 'Groups')} and ${plural(resources, 'Resource', 'Resources')}`;
  else if (groups) what = plural(groups, 'Group', 'Groups');
  else if (resources) what = plural(resources, 'Resource', 'Resources');
  else what = 'selection';
  return `Duplicates in ${what}, ${date}`;
}

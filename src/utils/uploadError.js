/**
 * Parse an upload error response from POST /v1/resource into a user-friendly
 * message and, when the server reported a content-hash collision, the id of the
 * resource it collided with.
 *
 * The backend returns a structured envelope with one entry per file:
 *   { "error": "...", "details": [{ "error": "...", "existingResourceId": 52 }] }
 *
 * Both upload paths send one file per request, so `details[0]` is that file's
 * own outcome.
 *
 * @param {string} responseText  Raw response body
 * @param {number} statusCode    HTTP status code
 * @returns {{ message: string, resourceId: number|null }}
 */
export function parseUploadError(responseText, statusCode) {
  let message = responseText || `Upload failed (HTTP ${statusCode})`;
  let resourceId = null;

  try {
    const json = JSON.parse(responseText);

    const detail = json.details?.[0];
    if (detail) {
      message = detail.error;
      if (detail.existingResourceId != null) {
        resourceId = detail.existingResourceId;
      }
    } else if (json.error) {
      message = json.error;
    }
  } catch (_) {
    // Not JSON – use raw text as-is
  }

  // Capitalise first letter for display
  if (message.length > 0) {
    message = message.charAt(0).toUpperCase() + message.slice(1);
  }

  return { message, resourceId };
}

import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

/**
 * Regression tests for HTTP status codes in version and download job APIs.
 *
 * Bug R9-A-001: Version API handlers return 500 for client errors that
 *   should be 400 or 404.
 * Bug R9-A-002: Download submit handler returns 503 for the client
 *   validation error "no valid URLs provided" instead of 400.
 */
test.describe('Version API HTTP status codes', () => {
  test('GET /v1/resource/versions returns 404 for non-existent resource', async ({
    request,
    baseURL,
  }) => {
    const response = await request.get(
      `${baseURL}/v1/resource/versions?resourceId=99999`
    );
    // Should be 404 because the resource doesn't exist, not 500
    expect(response.status()).toBe(404);
    const body = await response.json();
    expect(body.error).toContain('not found');
  });

  test('POST /v1/resource/versions returns 404 for non-existent resource', async ({
    request,
    baseURL,
  }) => {
    const testFilePath = path.join(__dirname, '../test-assets/sample-document.txt');
    const fs = await import('fs');
    const fileBuffer = fs.readFileSync(testFilePath);

    const response = await request.post(
      `${baseURL}/v1/resource/versions?resourceId=99999`,
      {
        multipart: {
          file: {
            name: 'test.txt',
            mimeType: 'text/plain',
            buffer: fileBuffer,
          },
        },
      }
    );
    // Should be 404 because the resource doesn't exist
    expect(response.status()).toBe(404);
  });

  test('POST /v1/resource/version/restore returns 404 for non-existent version', async ({
    request,
    baseURL,
    apiClient,
  }) => {
    // Create a resource first
    const testFilePath = path.join(__dirname, '../test-assets/sample-document.txt');
    const resource = await apiClient.createResource({
      filePath: testFilePath,
      name: `Status Test Resource ${Date.now()}`,
    });

    const response = await request.post(
      `${baseURL}/v1/resource/version/restore`,
      {
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        data: `resourceId=${resource.ID}&versionId=99999`,
      }
    );
    // Should be 404, not 500
    expect(response.status()).toBe(404);

    // Cleanup
    await apiClient.deleteResource(resource.ID);
  });

  test('DELETE /v1/resource/version returns 409 for current version', async ({
    request,
    baseURL,
    apiClient,
  }) => {
    // Create a resource
    const testFilePath = path.join(__dirname, '../test-assets/sample-document.txt');
    const resource = await apiClient.createResource({
      filePath: testFilePath,
      name: `Status Test Current ${Date.now()}`,
    });

    // Get versions to find the current one
    const listResp = await request.get(
      `${baseURL}/v1/resource/versions?resourceId=${resource.ID}`
    );
    const versions = await listResp.json();
    const currentVersion = versions[0]; // Only version, which is current

    const response = await request.delete(
      `${baseURL}/v1/resource/version?resourceId=${resource.ID}&versionId=${currentVersion.id}`
    );
    // Should be 409 Conflict (can't delete current), not 500
    expect(response.status()).toBe(409);

    // Cleanup
    await apiClient.deleteResource(resource.ID);
  });

  test('DELETE /v1/resource/version returns 404 for non-existent version', async ({
    request,
    baseURL,
    apiClient,
  }) => {
    const testFilePath = path.join(__dirname, '../test-assets/sample-document.txt');
    const resource = await apiClient.createResource({
      filePath: testFilePath,
      name: `Status Test NotFound ${Date.now()}`,
    });

    const response = await request.delete(
      `${baseURL}/v1/resource/version?resourceId=${resource.ID}&versionId=99999`
    );
    // Should be 404, not 500
    expect(response.status()).toBe(404);

    // Cleanup
    await apiClient.deleteResource(resource.ID);
  });

  test('DELETE /v1/resource/version returns 400 for wrong resource', async ({
    request,
    baseURL,
    apiClient,
  }) => {
    // Create two resources
    const testFilePath = path.join(__dirname, '../test-assets/sample-document.txt');
    const resource1 = await apiClient.createResource({
      filePath: testFilePath,
      name: `Status Test Res1 ${Date.now()}`,
    });
    const testFilePath2 = path.join(__dirname, '../test-assets/sample-image-10.png');
    const resource2 = await apiClient.createResource({
      filePath: testFilePath2,
      name: `Status Test Res2 ${Date.now()}`,
    });

    // Get version of resource1
    const listResp = await request.get(
      `${baseURL}/v1/resource/versions?resourceId=${resource1.ID}`
    );
    const versions = await listResp.json();

    // Try to delete resource1's version using resource2's ID
    const response = await request.delete(
      `${baseURL}/v1/resource/version?resourceId=${resource2.ID}&versionId=${versions[0].id}`
    );
    // Should be 400, not 500
    expect(response.status()).toBe(400);

    // Cleanup
    await apiClient.deleteResource(resource1.ID);
    await apiClient.deleteResource(resource2.ID);
  });

  test('GET /v1/resource/versions/compare returns 404 for non-existent version', async ({
    request,
    baseURL,
    apiClient,
  }) => {
    const testFilePath = path.join(__dirname, '../test-assets/sample-document.txt');
    const resource = await apiClient.createResource({
      filePath: testFilePath,
      name: `Status Test Compare ${Date.now()}`,
    });

    // Get version IDs
    const listResp = await request.get(
      `${baseURL}/v1/resource/versions?resourceId=${resource.ID}`
    );
    const versions = await listResp.json();

    const response = await request.get(
      `${baseURL}/v1/resource/versions/compare?resourceId=${resource.ID}&v1=${versions[0].id}&v2=99999`
    );
    // Should be 404, not 500
    expect(response.status()).toBe(404);

    // Cleanup
    await apiClient.deleteResource(resource.ID);
  });

  test('POST /v1/resource/versions/cleanup returns 404 for non-existent resource', async ({
    request,
    baseURL,
  }) => {
    const response = await request.post(
      `${baseURL}/v1/resource/versions/cleanup`,
      {
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        data: 'resourceId=99999&keepLast=1&dryRun=true',
      }
    );
    // Should be 404, not 500
    expect(response.status()).toBe(404);
  });
});

test.describe('Download job API HTTP status codes', () => {
  test('POST /v1/jobs/download/submit returns 400 for whitespace-only URLs', async ({
    request,
    baseURL,
  }) => {
    const response = await request.post(
      `${baseURL}/v1/jobs/download/submit`,
      {
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        data: 'URL=%20%20%20',
      }
    );
    // Should be 400 (invalid input), not 503 (service unavailable)
    expect(response.status()).toBe(400);
    const body = await response.json();
    expect(body.error).toContain('no valid URLs');
  });

  test('POST /v1/jobs/download/submit returns 400 for newline-only URL', async ({
    request,
    baseURL,
  }) => {
    const response = await request.post(
      `${baseURL}/v1/jobs/download/submit`,
      {
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        data: `URL=${encodeURIComponent('\n\n')}`,
      }
    );
    // Should be 400, not 503
    expect(response.status()).toBe(400);
  });
});

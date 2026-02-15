import { APIRequestContext, APIResponse } from '@playwright/test';

export interface Entity {
  ID: number;
  Name: string;
  Description?: string;
}

export type Tag = Entity;

export interface Category extends Entity {
  CustomHeader?: string;
  CustomSidebar?: string;
  CustomSummary?: string;
  CustomAvatar?: string;
  MetaSchema?: string;
}

export interface ResourceCategory extends Entity {
  CustomHeader?: string;
  CustomSidebar?: string;
  CustomSummary?: string;
  CustomAvatar?: string;
  MetaSchema?: string;
}

export interface NoteType extends Entity {
  CustomHeader?: string;
  CustomSidebar?: string;
  CustomSummary?: string;
  CustomAvatar?: string;
}

export interface Group extends Entity {
  URL?: string;
  CategoryId?: number;
  OwnerId?: number;
}

export interface Note extends Entity {
  StartDate?: string;
  EndDate?: string;
  OwnerId?: number;
  NoteTypeId?: number;
  ShareToken?: string;
}

export interface Query extends Entity {
  Text: string;
  Template?: string;
}

export interface RelationType extends Entity {
  FromCategoryId?: number;
  ToCategoryId?: number;
}

export interface Relation extends Entity {
  FromGroupId: number;
  ToGroupId: number;
  RelationTypeId: number;
}

export interface Series {
  ID: number;
  Name: string;
  Slug: string;
  Meta: Record<string, unknown>;
  Resources?: Resource[];
}

export interface Resource extends Entity {
  Hash?: string;
  ContentType?: string;
  Meta?: Record<string, unknown>;
  seriesId?: number;
  ownMeta?: Record<string, unknown>;
}

export interface SearchResult {
  ID: number;
  Name: string;
  Type: string;
  Description?: string;
  URL?: string;
}

export interface NoteBlock {
  id: number;
  createdAt: string;
  updatedAt: string;
  noteId: number;
  type: string;
  position: string;
  content: Record<string, unknown>;
  state: Record<string, unknown>;
}

export class ApiClient {
  constructor(
    private request: APIRequestContext,
    private baseUrl: string
  ) {}

  private async handleResponse<T>(response: APIResponse): Promise<T> {
    if (!response.ok()) {
      const text = await response.text();
      throw new Error(`API error ${response.status()}: ${text}`);
    }
    return response.json();
  }

  private async handleVoidResponse(response: APIResponse): Promise<void> {
    if (!response.ok()) {
      const text = await response.text();
      throw new Error(`API error ${response.status()}: ${text}`);
    }
  }

  /**
   * Retry a request function on transient SQLite "database is locked" errors.
   * These occur when parallel test workers contend for the same SQLite database.
   */
  private async withRetry<T>(fn: () => Promise<T>, maxRetries = 5): Promise<T> {
    for (let attempt = 0; attempt <= maxRetries; attempt++) {
      const start = Date.now();
      try {
        return await fn();
      } catch (err) {
        const elapsed = Date.now() - start;
        const msg = err instanceof Error ? err.message : String(err);
        if (attempt < maxRetries && msg.includes('database is locked')) {
          // Exponential backoff with jitter to avoid thundering herd
          const baseDelay = 500 * Math.pow(2, attempt);
          const jitter = Math.random() * 300;
          const delay = baseDelay + jitter;
          console.log(`[withRetry] attempt ${attempt + 1}/${maxRetries} failed after ${elapsed}ms (database locked), retrying in ${Math.round(delay)}ms`);
          await new Promise(r => setTimeout(r, delay));
          continue;
        }
        throw err;
      }
    }
    throw new Error('withRetry: unreachable');
  }

  /** POST with automatic retry on transient SQLite errors. */
  private async postRetry<T>(url: string, options?: Parameters<APIRequestContext['post']>[1]): Promise<T> {
    return this.withRetry(async () => {
      const response = await this.request.post(url, options);
      return this.handleResponse<T>(response);
    });
  }

  /** POST (void response) with automatic retry on transient SQLite errors. */
  private async postVoidRetry(url: string, options?: Parameters<APIRequestContext['post']>[1]): Promise<void> {
    return this.withRetry(async () => {
      const response = await this.request.post(url, options);
      return this.handleVoidResponse(response);
    });
  }

  /** DELETE with automatic retry on transient SQLite errors. */
  private async deleteRetry(url: string, options?: Parameters<APIRequestContext['delete']>[1]): Promise<void> {
    return this.withRetry(async () => {
      const response = await this.request.delete(url, options);
      return this.handleVoidResponse(response);
    });
  }

  // Tag operations
  async createTag(name: string, description?: string): Promise<Tag> {
    const formData = new URLSearchParams();
    formData.append('name', name);
    if (description) formData.append('Description', description);

    return this.postRetry<Tag>(`${this.baseUrl}/v1/tag`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async deleteTag(id: number): Promise<void> {
    return this.postVoidRetry(`${this.baseUrl}/v1/tag/delete?Id=${id}`);
  }

  async getTags(): Promise<Tag[]> {
    const response = await this.request.get(`${this.baseUrl}/v1/tags`);
    return this.handleResponse<Tag[]>(response);
  }

  // Category operations
  async createCategory(
    name: string,
    description?: string,
    options?: Partial<Category>
  ): Promise<Category> {
    const formData = new URLSearchParams();
    formData.append('name', name);
    if (description) formData.append('Description', description);
    if (options?.CustomHeader) formData.append('CustomHeader', options.CustomHeader);
    if (options?.CustomSidebar) formData.append('CustomSidebar', options.CustomSidebar);
    if (options?.CustomSummary) formData.append('CustomSummary', options.CustomSummary);
    if (options?.CustomAvatar) formData.append('CustomAvatar', options.CustomAvatar);
    if (options?.MetaSchema) formData.append('MetaSchema', options.MetaSchema);

    return this.postRetry<Category>(`${this.baseUrl}/v1/category`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async deleteCategory(id: number): Promise<void> {
    return this.postVoidRetry(`${this.baseUrl}/v1/category/delete?Id=${id}`);
  }

  async getCategories(): Promise<Category[]> {
    const response = await this.request.get(`${this.baseUrl}/v1/categories`);
    return this.handleResponse<Category[]>(response);
  }

  // ResourceCategory operations
  async createResourceCategory(
    name: string,
    description?: string,
    options?: {
      CustomHeader?: string;
      CustomSidebar?: string;
      CustomSummary?: string;
      CustomAvatar?: string;
      MetaSchema?: string;
    }
  ): Promise<ResourceCategory> {
    const formData = new URLSearchParams();
    formData.append('name', name);
    if (description) formData.append('Description', description);
    if (options?.CustomHeader) formData.append('CustomHeader', options.CustomHeader);
    if (options?.CustomSidebar) formData.append('CustomSidebar', options.CustomSidebar);
    if (options?.CustomSummary) formData.append('CustomSummary', options.CustomSummary);
    if (options?.CustomAvatar) formData.append('CustomAvatar', options.CustomAvatar);
    if (options?.MetaSchema) formData.append('MetaSchema', options.MetaSchema);

    return this.postRetry<ResourceCategory>(`${this.baseUrl}/v1/resourceCategory`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async deleteResourceCategory(id: number): Promise<void> {
    return this.postVoidRetry(`${this.baseUrl}/v1/resourceCategory/delete?Id=${id}`);
  }

  async getResourceCategories(): Promise<ResourceCategory[]> {
    const response = await this.request.get(`${this.baseUrl}/v1/resourceCategories`);
    return this.handleResponse<ResourceCategory[]>(response);
  }

  // NoteType operations
  async createNoteType(name: string, description?: string): Promise<NoteType> {
    const formData = new URLSearchParams();
    formData.append('name', name);
    if (description) formData.append('Description', description);

    return this.postRetry<NoteType>(`${this.baseUrl}/v1/note/noteType`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async deleteNoteType(id: number): Promise<void> {
    return this.postVoidRetry(`${this.baseUrl}/v1/note/noteType/delete?Id=${id}`);
  }

  async getNoteTypes(): Promise<NoteType[]> {
    const response = await this.request.get(`${this.baseUrl}/v1/note/noteTypes`);
    return this.handleResponse<NoteType[]>(response);
  }

  // Group operations
  async createGroup(data: {
    name: string;
    description?: string;
    categoryId: number;
    ownerId?: number;
    tags?: number[];
    groups?: number[];
    url?: string;
  }): Promise<Group> {
    const formData = new URLSearchParams();
    formData.append('name', data.name);
    if (data.description) formData.append('Description', data.description);
    formData.append('categoryId', data.categoryId.toString());
    if (data.ownerId) formData.append('ownerId', data.ownerId.toString());
    if (data.url) formData.append('URL', data.url);
    if (data.tags) {
      data.tags.forEach(tagId => formData.append('tags', tagId.toString()));
    }
    if (data.groups) {
      data.groups.forEach(groupId => formData.append('groups', groupId.toString()));
    }

    return this.postRetry<Group>(`${this.baseUrl}/v1/group`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async deleteGroup(id: number): Promise<void> {
    return this.postVoidRetry(`${this.baseUrl}/v1/group/delete?Id=${id}`);
  }

  async getGroups(): Promise<Group[]> {
    const response = await this.request.get(`${this.baseUrl}/v1/groups`);
    return this.handleResponse<Group[]>(response);
  }

  async getGroup(id: number): Promise<Group> {
    const response = await this.request.get(`${this.baseUrl}/v1/group?id=${id}`);
    return this.handleResponse<Group>(response);
  }

  // Note operations
  async createNote(data: {
    name: string;
    description?: string;
    ownerId?: number;
    noteTypeId?: number;
    tags?: number[];
    groups?: number[];
    resources?: number[];
    startDate?: string;
    endDate?: string;
  }): Promise<Note> {
    const formData = new URLSearchParams();
    formData.append('Name', data.name);
    if (data.description) formData.append('Description', data.description);
    if (data.ownerId) formData.append('ownerId', data.ownerId.toString());
    if (data.noteTypeId) formData.append('NoteTypeId', data.noteTypeId.toString());
    if (data.startDate) formData.append('startDate', data.startDate);
    if (data.endDate) formData.append('endDate', data.endDate);
    if (data.tags) {
      data.tags.forEach(tagId => formData.append('tags', tagId.toString()));
    }
    if (data.groups) {
      data.groups.forEach(groupId => formData.append('groups', groupId.toString()));
    }
    if (data.resources) {
      data.resources.forEach(resourceId => formData.append('Resources', resourceId.toString()));
    }

    return this.postRetry<Note>(`${this.baseUrl}/v1/note`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async deleteNote(id: number): Promise<void> {
    return this.postVoidRetry(`${this.baseUrl}/v1/note/delete?Id=${id}`);
  }

  async getNotes(): Promise<Note[]> {
    const response = await this.request.get(`${this.baseUrl}/v1/notes`);
    return this.handleResponse<Note[]>(response);
  }

  async getNote(id: number): Promise<Note> {
    const response = await this.request.get(`${this.baseUrl}/v1/note?id=${id}`);
    return this.handleResponse<Note>(response);
  }

  async updateNote(id: number, data: {
    name?: string;
    description?: string;
    ownerId?: number;
    noteTypeId?: number;
    tags?: number[];
    groups?: number[];
    startDate?: string;
    endDate?: string;
  }): Promise<Note> {
    const formData = new URLSearchParams();
    formData.append('ID', id.toString());
    if (data.name) formData.append('Name', data.name);
    if (data.description !== undefined) formData.append('Description', data.description);
    if (data.ownerId) formData.append('ownerId', data.ownerId.toString());
    if (data.noteTypeId) formData.append('NoteTypeId', data.noteTypeId.toString());
    if (data.startDate) formData.append('startDate', data.startDate);
    if (data.endDate) formData.append('endDate', data.endDate);
    if (data.tags) {
      data.tags.forEach(tagId => formData.append('tags', tagId.toString()));
    }
    if (data.groups) {
      data.groups.forEach(groupId => formData.append('groups', groupId.toString()));
    }

    return this.postRetry<Note>(`${this.baseUrl}/v1/note`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  // Query operations
  async createQuery(data: {
    name: string;
    text: string;
    description?: string;
    template?: string;
  }): Promise<Query> {
    const formData = new URLSearchParams();
    formData.append('name', data.name);
    formData.append('Text', data.text);
    if (data.description) formData.append('Description', data.description);
    if (data.template) formData.append('Template', data.template);

    return this.postRetry<Query>(`${this.baseUrl}/v1/query`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async deleteQuery(id: number): Promise<void> {
    return this.postVoidRetry(`${this.baseUrl}/v1/query/delete?Id=${id}`);
  }

  async getQueries(): Promise<Query[]> {
    const response = await this.request.get(`${this.baseUrl}/v1/queries`);
    return this.handleResponse<Query[]>(response);
  }

  // RelationType operations
  async createRelationType(data: {
    name: string;
    description?: string;
    fromCategoryId?: number;
    toCategoryId?: number;
  }): Promise<RelationType> {
    const formData = new URLSearchParams();
    formData.append('name', data.name);
    if (data.description) formData.append('Description', data.description);
    // The Go backend uses FromCategory/ToCategory (not FromCategoryId/ToCategoryId)
    if (data.fromCategoryId) formData.append('FromCategory', data.fromCategoryId.toString());
    if (data.toCategoryId) formData.append('ToCategory', data.toCategoryId.toString());

    return this.postRetry<RelationType>(`${this.baseUrl}/v1/relationType`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async deleteRelationType(id: number): Promise<void> {
    return this.postVoidRetry(`${this.baseUrl}/v1/relationType/delete?Id=${id}`);
  }

  async getRelationTypes(): Promise<RelationType[]> {
    const response = await this.request.get(`${this.baseUrl}/v1/relationTypes`);
    return this.handleResponse<RelationType[]>(response);
  }

  // Relation operations
  async createRelation(data: {
    name?: string;
    description?: string;
    fromGroupId: number;
    toGroupId: number;
    relationTypeId: number;
  }): Promise<Relation> {
    // Send as JSON (like Go API tests do) to get JSON response
    const jsonBody = {
      Name: data.name || '',
      Description: data.description || '',
      FromGroupId: data.fromGroupId,
      ToGroupId: data.toGroupId,
      GroupRelationTypeId: data.relationTypeId,
    };

    return this.postRetry<Relation>(`${this.baseUrl}/v1/relation`, {
      headers: {
        'Content-Type': 'application/json',
      },
      data: JSON.stringify(jsonBody),
    });
  }

  async deleteRelation(id: number): Promise<void> {
    return this.postVoidRetry(`${this.baseUrl}/v1/relation/delete?Id=${id}`);
  }

  // Search
  async search(query: string, limit = 15): Promise<SearchResult[]> {
    const response = await this.request.get(
      `${this.baseUrl}/v1/search?q=${encodeURIComponent(query)}&limit=${limit}`
    );
    return this.handleResponse<SearchResult[]>(response);
  }

  // Bulk operations
  async addTagsToGroups(groupIds: number[], tagIds: number[]): Promise<void> {
    const formData = new URLSearchParams();
    // BulkEditQuery expects ID[] for group IDs and EditedId[] for tag IDs
    groupIds.forEach(id => formData.append('ID', id.toString()));
    tagIds.forEach(id => formData.append('EditedId', id.toString()));

    return this.postVoidRetry(`${this.baseUrl}/v1/groups/addTags`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async removeTagsFromGroups(groupIds: number[], tagIds: number[]): Promise<void> {
    const formData = new URLSearchParams();
    // BulkEditQuery expects ID[] for group IDs and EditedId[] for tag IDs
    groupIds.forEach(id => formData.append('ID', id.toString()));
    tagIds.forEach(id => formData.append('EditedId', id.toString()));

    return this.postVoidRetry(`${this.baseUrl}/v1/groups/removeTags`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async bulkDeleteGroups(groupIds: number[]): Promise<void> {
    const formData = new URLSearchParams();
    // BulkQuery expects ID[] for group IDs
    groupIds.forEach(id => formData.append('ID', id.toString()));

    return this.postVoidRetry(`${this.baseUrl}/v1/groups/delete`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  // Resource operations
  async getResources(): Promise<{ ID: number; Name: string; Hash: string; ContentType: string }[]> {
    const response = await this.request.get(`${this.baseUrl}/v1/resources`);
    return this.handleResponse(response);
  }

  async getResourcesPaginated(page: number, pageSize = 50): Promise<{ ID: number; Name: string; Hash: string; ContentType: string }[]> {
    const response = await this.request.get(`${this.baseUrl}/v1/resources?page=${page}&pageSize=${pageSize}`);
    return this.handleResponse(response);
  }

  async createResource(data: {
    filePath: string;
    name: string;
    description?: string;
    ownerId?: number;
    tags?: number[];
    resourceCategoryId?: number;
    seriesSlug?: string;
    seriesId?: number;
    meta?: string;
  }): Promise<{ ID: number; Name: string; ContentType: string }> {
    const fs = await import('fs');
    const pathModule = await import('path');

    const fileBuffer = fs.readFileSync(data.filePath);
    const fileName = pathModule.basename(data.filePath);

    // Build multipart object - the field name must be "resource" to match server
    type MultipartValue = string | number | boolean | {
      name: string;
      mimeType: string;
      buffer: Buffer;
    };
    const multipartData: Record<string, MultipartValue> = {
      resource: {
        name: fileName,
        mimeType: 'image/png',
        buffer: fileBuffer,
      },
      Name: data.name,
    };

    if (data.description) {
      multipartData.Description = data.description;
    }
    if (data.ownerId) {
      multipartData.OwnerId = data.ownerId.toString();
    }
    if (data.resourceCategoryId) {
      multipartData.ResourceCategoryId = data.resourceCategoryId.toString();
    }
    if (data.seriesSlug) {
      multipartData.SeriesSlug = data.seriesSlug;
    }
    if (data.seriesId) {
      multipartData.SeriesId = data.seriesId.toString();
    }
    if (data.meta) {
      multipartData.Meta = data.meta;
    }

    return this.withRetry(async () => {
      const response = await this.request.post(`${this.baseUrl}/v1/resource`, {
        multipart: multipartData,
      });
      // The API returns an array of resources, extract the first one
      const resources = await this.handleResponse<{ ID: number; Name: string; ContentType: string }[]>(response);
      if (!resources || resources.length === 0) {
        throw new Error('No resource returned from API');
      }
      return resources[0];
    });
  }

  async deleteResource(id: number): Promise<void> {
    return this.postVoidRetry(`${this.baseUrl}/v1/resource/delete?Id=${id}`);
  }

  async addTagsToResources(resourceIds: number[], tagIds: number[]): Promise<void> {
    const formData = new URLSearchParams();
    resourceIds.forEach(id => formData.append('ID', id.toString()));
    tagIds.forEach(id => formData.append('EditedId', id.toString()));

    const response = await this.request.post(`${this.baseUrl}/v1/resources/addTags`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
    await this.handleVoidResponse(response);
  }

  async removeTagsFromResources(resourceIds: number[], tagIds: number[]): Promise<void> {
    const formData = new URLSearchParams();
    resourceIds.forEach(id => formData.append('ID', id.toString()));
    tagIds.forEach(id => formData.append('EditedId', id.toString()));

    const response = await this.request.post(`${this.baseUrl}/v1/resources/removeTags`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
    await this.handleVoidResponse(response);
  }

  // Block operations
  async createBlock(
    noteId: number,
    type: string,
    position: string,
    content: Record<string, unknown>
  ): Promise<NoteBlock> {
    return this.postRetry<NoteBlock>(`${this.baseUrl}/v1/note/block`, {
      headers: { 'Content-Type': 'application/json' },
      data: JSON.stringify({
        noteId,
        type,
        position,
        content,
      }),
    });
  }

  async getBlocks(noteId: number): Promise<NoteBlock[]> {
    const response = await this.request.get(`${this.baseUrl}/v1/note/blocks?noteId=${noteId}`);
    return this.handleResponse<NoteBlock[]>(response);
  }

  async getBlock(blockId: number): Promise<NoteBlock> {
    const response = await this.request.get(`${this.baseUrl}/v1/note/block?id=${blockId}`);
    return this.handleResponse<NoteBlock>(response);
  }

  async updateBlockContent(
    blockId: number,
    content: Record<string, unknown>
  ): Promise<NoteBlock> {
    return this.withRetry(async () => {
      const response = await this.request.put(`${this.baseUrl}/v1/note/block?id=${blockId}`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ content }),
      });
      return this.handleResponse<NoteBlock>(response);
    });
  }

  async updateBlockState(
    blockId: number,
    state: Record<string, unknown>
  ): Promise<NoteBlock> {
    return this.withRetry(async () => {
      const response = await this.request.patch(`${this.baseUrl}/v1/note/block/state?id=${blockId}`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ state }),
      });
      return this.handleResponse<NoteBlock>(response);
    });
  }

  async deleteBlock(blockId: number): Promise<void> {
    return this.deleteRetry(`${this.baseUrl}/v1/note/block?id=${blockId}`);
  }

  async reorderBlocks(noteId: number, positions: Record<number, string>): Promise<void> {
    return this.postVoidRetry(`${this.baseUrl}/v1/note/blocks/reorder`, {
      headers: { 'Content-Type': 'application/json' },
      data: JSON.stringify({ noteId, positions }),
    });
  }

  // Note sharing operations
  async shareNote(noteId: number): Promise<{ token: string }> {
    const data = await this.postRetry<{ shareToken: string; shareUrl: string }>(
      `${this.baseUrl}/v1/note/share?noteId=${noteId}`
    );
    return { token: data.shareToken };
  }

  async unshareNote(noteId: number): Promise<void> {
    return this.deleteRetry(`${this.baseUrl}/v1/note/share?noteId=${noteId}`);
  }

  async getSharedNotes(): Promise<Note[]> {
    const response = await this.request.get(`${this.baseUrl}/v1/notes?Shared=1`);
    return this.handleResponse<Note[]>(response);
  }

  // Series operations
  async getSeries(id: number): Promise<Series> {
    const response = await this.request.get(`${this.baseUrl}/v1/series?id=${id}`);
    return this.handleResponse<Series>(response);
  }

  async updateSeries(id: number, data: { name?: string; meta?: string }): Promise<Series> {
    const formData = new URLSearchParams();
    formData.append('ID', id.toString());
    if (data.name) formData.append('Name', data.name);
    if (data.meta) formData.append('Meta', data.meta);

    return this.postRetry<Series>(`${this.baseUrl}/v1/series`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }

  async deleteSeries(id: number): Promise<void> {
    return this.postVoidRetry(`${this.baseUrl}/v1/series/delete?Id=${id}`);
  }

  async removeResourceFromSeries(resourceId: number): Promise<void> {
    return this.postVoidRetry(`${this.baseUrl}/v1/resource/removeSeries?Id=${resourceId}`);
  }

  // Get resource details including hash and series
  async getResource(id: number): Promise<Resource> {
    const response = await this.request.get(`${this.baseUrl}/v1/resource?Id=${id}`);
    return this.handleResponse<Resource>(response);
  }

  // Add resources to a note by updating the note
  async addResourcesToNote(noteId: number, resourceIds: number[]): Promise<Note> {
    // First get the note to preserve its name (uses existing getNote method)
    const existingNote = await this.getNote(noteId);

    const formData = new URLSearchParams();
    formData.append('ID', noteId.toString());
    formData.append('Name', existingNote.Name);
    resourceIds.forEach(id => formData.append('Resources', id.toString()));

    const response = await this.request.post(`${this.baseUrl}/v1/note`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
    return this.handleResponse<Note>(response);
  }
}

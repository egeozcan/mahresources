import type { EntityProfileName } from '../../selector/entityFieldProfiles';

type Base = { key: string; label: string; section: 'basic' | 'advanced' };
export type PickerFilter = Base & (
 | { kind: 'entity'; entity: EntityProfileName; multiple: boolean }
 | { kind: 'text' | 'date' | 'number' | 'checkbox' | 'metadata' }
);
const scalar = (key: string, label: string, kind: 'text' | 'date' | 'number' | 'checkbox' | 'metadata' = 'text', section: Base['section'] = 'advanced'): PickerFilter => ({ key, label, kind, section });
const entity = (key: string, label: string, entity: EntityProfileName, multiple = true, section: Base['section'] = 'basic'): PickerFilter => ({ key, label, kind: 'entity', entity, multiple, section });
const name = scalar('Name', 'Name', 'text', 'basic');
const description = scalar('Description', 'Description');
const created = [scalar('CreatedBefore','Created before','date'),scalar('CreatedAfter','Created after','date')];
const updated = [scalar('UpdatedBefore','Updated before','date'),scalar('UpdatedAfter','Updated after','date')];
const meta = scalar('MetaQuery','Metadata','metadata');
const mrql = scalar('MRQL','MRQL filter expression');
const tags = entity('Tags','Tags','tag');
const owner = entity('OwnerId','Owner','group',false);

/** Predicate fields, not a second query language. Names match the existing list
 * DTOs; sorting and page size are deliberately owned by the browse service.
 */
export const entityFilterConfigs: Record<EntityProfileName, readonly PickerFilter[]> = {
 resource: [name,entity('ResourceCategoryId','Resource category','resourceCategory',false),owner,tags,
  scalar('ContentType','Content type','text','basic'),description,
  entity('Groups','Groups','group',true,'advanced'),entity('Notes','Notes','note',true,'advanced'),
  entity('SeriesId','Series','series',false,'advanced'),
  scalar('OriginalName','Original name'),scalar('Hash','Hash'),scalar('OriginalLocation','Original location'),
  scalar('IncludeSubgroups','Include subgroups','checkbox'),scalar('ShowWithoutOwner','Without owner','checkbox'),
  ...created,...updated,...['MinWidth','MaxWidth','MinHeight','MaxHeight'].map(key=>scalar(key,key.replace(/(Width|Height)/,' $1'),'number')),
  scalar('ShowWithSimilar','Has similar resources','checkbox'),scalar('Untagged','Untagged','checkbox'),meta,mrql],
 group: [name,entity('Categories','Categories','category'),owner,tags,description,scalar('URL','URL'),
  scalar('SearchParentsForName','Search parents for name','checkbox'),scalar('SearchChildrenForName','Search children for name','checkbox'),
  scalar('SearchParentsForTags','Search parents for tags','checkbox'),scalar('SearchChildrenForTags','Search children for tags','checkbox'),
  entity('Notes','Notes','note',true,'advanced'),entity('Resources','Resources','resource',true,'advanced'),entity('Groups','Related groups','group',true,'advanced'),
  entity('RelationTypeId','Relation type','relationType',false,'advanced'),scalar('RelationSide','Relation side (0: from, 1: to)','number'),
  ...created,...updated,meta,mrql],
 note: [name,entity('NoteTypeId','Note type','noteType',false),owner,tags,description,
  entity('Groups','Groups','group',true,'advanced'),scalar('StartDateBefore','Starts before','date'),scalar('StartDateAfter','Starts after','date'),
  scalar('EndDateBefore','Ends before','date'),scalar('EndDateAfter','Ends after','date'),scalar('Shared','Shared','checkbox'),meta,mrql],
 category: [name,description,...created],
 noteType: [name,description],
 resourceCategory: [name,description],
 tag: [name,description,...created],
 query: [name,scalar('Text','Query text'),...created],
 relationType: [name,description,entity('FromCategory','From category','category',false),entity('ToCategory','To category','category',false)],
 series: [name,scalar('Slug','Slug'),...created],
};

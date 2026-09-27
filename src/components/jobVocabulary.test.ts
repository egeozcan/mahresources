import { describe, expect, test } from 'vitest';
import { blockedReasonText, failureClassLabel, kindLabel, originLabel } from './jobVocabulary.js';

// The same table the server reads (server/jobview/job_vocabulary.json), with the
// same fallbacks (server/jobview/vocabulary.go).
describe('Job vocabulary', () => {
    test('names Kinds, origins and failure classes in words', () => {
        expect(kindLabel('remote-download')).toBe('Download');
        expect(kindLabel('deferred-download')).toBe('Scheduled download');
        expect(kindLabel('plugin-command-import')).toBe('Command output import');
        expect(originLabel('api')).toBe('Web page or API');
        expect(failureClassLabel('internal')).toBe('Internal error');
    });

    test('keeps an identifier it does not know', () => {
        expect(kindLabel('a-plugin-kind')).toBe('a-plugin-kind');
        expect(originLabel('elsewhere')).toBe('elsewhere');
        expect(kindLabel(undefined)).toBe('');
    });

    test('reads a blocked reason as a sentence, and an unknown one from its code', () => {
        expect(blockedReasonText('role-refused')).toBe('The account that asked for it may no longer run it.');
        expect(blockedReasonText('source-row-missing')).toBe('Source row missing.');
        expect(blockedReasonText('')).toBe('');
        expect(blockedReasonText(undefined)).toBe('');
    });
});

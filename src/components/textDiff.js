import * as Diff from 'diff';

export function textDiff({ leftUrl, rightUrl }) {
  return {
    mode: 'unified',
    loading: true,
    error: null,
    leftText: '',
    rightText: '',
    unifiedDiff: [],
    splitLeft: [],
    splitRight: [],
    stats: { added: 0, removed: 0 },

    async init() {
      try {
        const [leftRes, rightRes] = await Promise.all([
          fetch(leftUrl),
          fetch(rightUrl)
        ]);

        if (!leftRes.ok || !rightRes.ok) {
          throw new Error('Failed to load files');
        }

        this.leftText = await leftRes.text();
        this.rightText = await rightRes.text();
        this.computeDiff();
      } catch (e) {
        this.error = e.message;
      } finally {
        this.loading = false;
      }
    },

    computeDiff() {
      const diff = Diff.diffLines(this.leftText, this.rightText);

      // Build unified diff
      this.unifiedDiff = [];
      this.splitLeft = [];
      this.splitRight = [];

      let leftNum = 0;
      let rightNum = 0;
      let added = 0;
      let removed = 0;

      for (const part of diff) {
        const lines = part.value.split('\n');
        // Remove last empty line from split
        if (lines[lines.length - 1] === '') {
          lines.pop();
        }

        for (const line of lines) {
          if (part.added) {
            rightNum++;
            added++;
            this.unifiedDiff.push({
              type: 'added',
              prefix: '+',
              content: line,
              leftNum: null,
              rightNum: rightNum
            });
            this.splitLeft.push({ num: null, content: '', changed: false });
            this.splitRight.push({ num: rightNum, content: line, changed: true });
          } else if (part.removed) {
            leftNum++;
            removed++;
            this.unifiedDiff.push({
              type: 'removed',
              prefix: '-',
              content: line,
              leftNum: leftNum,
              rightNum: null
            });
            this.splitLeft.push({ num: leftNum, content: line, changed: true });
            this.splitRight.push({ num: null, content: '', changed: false });
          } else {
            leftNum++;
            rightNum++;
            this.unifiedDiff.push({
              type: 'context',
              prefix: ' ',
              content: line,
              leftNum: leftNum,
              rightNum: rightNum
            });
            this.splitLeft.push({ num: leftNum, content: line, changed: false });
            this.splitRight.push({ num: rightNum, content: line, changed: false });
          }
        }
      }

      this.stats = { added, removed };
    }
  };
}

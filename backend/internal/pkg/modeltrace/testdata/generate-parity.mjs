// 仅离线读取指定版本的 ModelTrace 参考数据，生成 Go 移植的一致性测试样本。
// 不请求模型、不运行上游插件，不参与应用运行或日常 Go 测试。
import { readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = process.argv[2];
if (!root) throw new Error('请传入 ModelTrace 参考源码目录');
const { analyzeGlobalOutputs } = await import(pathToFileURL(path.join(root, 'codex-plugin/modeltrace-guard/scripts/fingerprint-core.mjs')));
const bank = JSON.parse(await readFile(path.join(root, 'data/unified_bank.json'), 'utf8'));
const rows = (await Promise.all(['gpt', 'claude'].map(async family =>
  (await readFile(path.join(root, `data/${family}_reference.jsonl`), 'utf8')).trim().split('\n').map(JSON.parse),
))).flat();
const cases = bank.models.map(model => {
  const selected = rows.filter(row => row.model_id === model.id && row.strict_valid).slice(0, 3);
  if (selected.length !== 3) throw new Error(`缺少 ${model.id} 的参考回答`);
  const outputs = selected.map(row => ({ text: row.text, expected_count: row.requested_count }));
  return {
    model: model.id,
    row_ids: selected.map(row => row.row_id),
    outputs,
    expected: [1, 2, 3].map(count => {
      const result = analyzeGlobalOutputs(outputs.slice(0, count), bank);
      return { ...result, valid_responses: count, margin: result.probability - result.results[1].probability };
    }),
  };
});
const fixture = { source_commit: 'df3a0f9d3e054c0dc02d6d586686db8daf8fa7c8', note: '仅验证既有参考数据上 Go 与原 JavaScript 数值一致，不代表独立检测准确率。', cases };
await writeFile(path.join(path.dirname(fileURLToPath(import.meta.url)), 'parity.json'), JSON.stringify(fixture, null, 2) + '\n');
process.stdout.write(`已生成 ${cases.length * 3} 个离线一致性案例。\n`);

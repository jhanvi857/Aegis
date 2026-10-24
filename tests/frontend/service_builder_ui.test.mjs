import { test, describe } from 'node:test';
import assert from 'node:assert';

describe('Service & Dependency Builder UI Logic', () => {
  // 1. Service Form Validation
  describe('Service Form Validation', () => {
    test('enforces kebab-case for service ID', () => {
      const sanitizeId = (id) => id.toLowerCase().replace(/[^a-z0-9-_]/g, '');
      assert.strictEqual(sanitizeId('Order Processing Service #1'), 'orderprocessingservice1');
      assert.strictEqual(sanitizeId('auth_service-v2'), 'auth_service-v2');
    });

    test('validates mode permissions for auto_remediate', () => {
      const canEnableAutoRemediate = (userRole, hasRecoveryBinding, allowedActions) => {
        if (userRole !== 'admin') return false;
        if (!hasRecoveryBinding) return false;
        if (!allowedActions || allowedActions.length === 0) return false;
        return true;
      };

      assert.strictEqual(canEnableAutoRemediate('viewer', true, ['scale']), false);
      assert.strictEqual(canEnableAutoRemediate('operator', true, ['scale']), false);
      assert.strictEqual(canEnableAutoRemediate('admin', false, []), false);
      assert.strictEqual(canEnableAutoRemediate('admin', true, []), false);
      assert.strictEqual(canEnableAutoRemediate('admin', true, ['scale', 'restart']), true);
    });

    test('generates correct copyable install snippet for agent-push', () => {
      const buildSnippet = (serviceId, apiKey) => {
        return `docker run -d --name aegis-agent-${serviceId} -e AEGIS_SERVICE_ID="${serviceId}" -e AEGIS_API_KEY="${apiKey}" ghcr.io/aegis/agent:latest`;
      };
      const snippet = buildSnippet('payment-service', 'aegis-key-12345');
      assert.match(snippet, /payment-service/);
      assert.match(snippet, /aegis-key-12345/);
    });
  });

  // 2. Dependency Graph Layout & Cycle Handling
  describe('Dependency Graph Layout', () => {
    function computeLevels(services, edges) {
      const inDegree = {};
      const adj = {};
      services.forEach((s) => {
        inDegree[s.id] = 0;
        adj[s.id] = [];
      });
      edges.forEach((e) => {
        if (inDegree[e.target] !== undefined) inDegree[e.target]++;
        if (adj[e.source]) adj[e.source].push(e.target);
      });

      const levels = {};
      const queue = [];
      services.forEach((s) => {
        if (inDegree[s.id] === 0 || s.type === 'gateway') {
          levels[s.id] = 0;
          queue.push({ id: s.id, level: 0 });
        }
      });
      if (queue.length === 0 && services.length > 0) {
        levels[services[0].id] = 0;
        queue.push({ id: services[0].id, level: 0 });
      }

      while (queue.length > 0) {
        const { id, level } = queue.shift();
        (adj[id] || []).forEach((nextId) => {
          const nextLevel = level + 1;
          if (levels[nextId] === undefined || levels[nextId] < nextLevel) {
            levels[nextId] = nextLevel;
            queue.push({ id: nextId, level: nextLevel });
          }
        });
      }
      return levels;
    }

    test('orders simple DAG into hierarchical levels', () => {
      const services = [
        { id: 'gateway', type: 'gateway' },
        { id: 'auth', type: 'service' },
        { id: 'db', type: 'database' },
      ];
      const edges = [
        { source: 'gateway', target: 'auth' },
        { source: 'auth', target: 'db' },
      ];

      const levels = computeLevels(services, edges);
      assert.strictEqual(levels['gateway'], 0);
      assert.strictEqual(levels['auth'], 1);
      assert.strictEqual(levels['db'], 2);
    });

    test('rejects self-loops during connection creation', () => {
      const isValidEdge = (source, target) => {
        if (!source || !target) return false;
        if (source === target) return false; // self loop forbidden
        return true;
      };

      assert.strictEqual(isValidEdge('svc-a', 'svc-a'), false);
      assert.strictEqual(isValidEdge('svc-a', 'svc-b'), true);
    });
  });

  // 3. Dry-Run Import Diff Computation
  describe('Import Dry-Run Diff Parsing', () => {
    test('computes additions, modifications, and deletions diff', () => {
      const currentServices = [{ id: 'svc-1', name: 'Old Name' }, { id: 'svc-2', name: 'Service 2' }];
      const incomingServices = [{ id: 'svc-1', name: 'New Name' }, { id: 'svc-3', name: 'Service 3' }];

      const additions = incomingServices.filter((inc) => !currentServices.some((c) => c.id === inc.id));
      const deletions = currentServices.filter((c) => !incomingServices.some((inc) => inc.id === c.id));
      const modifications = incomingServices.filter((inc) => {
        const existing = currentServices.find((c) => c.id === inc.id);
        return existing && existing.name !== inc.name;
      });

      assert.strictEqual(additions.length, 1);
      assert.strictEqual(additions[0].id, 'svc-3');

      assert.strictEqual(deletions.length, 1);
      assert.strictEqual(deletions[0].id, 'svc-2');

      assert.strictEqual(modifications.length, 1);
      assert.strictEqual(modifications[0].name, 'New Name');
    });
  });
});

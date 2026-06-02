const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

class FakeElement {
  constructor(id = '') {
    this.id = id;
    this.value = 'existing value';
    this.textContent = '';
    this.disabled = false;
    this.style = { display: '' };
    this.parentElement = null;
    this.children = [];
    this.classList = {
      add() {},
      remove() {},
      toggle() {},
    };
  }

  appendChild(child) {
    child.parentElement = this;
    this.children.push(child);
    return child;
  }

  querySelector() {
    return null;
  }

  querySelectorAll() {
    return [];
  }
}

function createKnowledgeScriptContext() {
  const windowListeners = new Map();
  const documentListeners = new Map();
  const elements = new Map();

  const getElement = (id) => {
    if (!elements.has(id)) {
      elements.set(id, new FakeElement(id));
    }
    return elements.get(id);
  };

  const saveButton = new FakeElement('knowledge-save-button');
  const cancelButton = new FakeElement('knowledge-cancel-button');

  const document = {
    body: new FakeElement('body'),
    createElement: (tagName) => new FakeElement(tagName),
    getElementById: getElement,
    querySelector: (selector) => {
      if (selector === '#knowledge-item-modal .modal-footer .btn-primary') return saveButton;
      if (selector === '#knowledge-item-modal .modal-footer .btn-secondary') return cancelButton;
      return null;
    },
    querySelectorAll: () => [],
    addEventListener: (type, handler) => {
      if (!documentListeners.has(type)) documentListeners.set(type, []);
      documentListeners.get(type).push(handler);
    },
  };

  const window = {
    addEventListener: (type, handler) => {
      if (!windowListeners.has(type)) windowListeners.set(type, []);
      windowListeners.get(type).push(handler);
    },
  };

  const context = {
    console,
    clearInterval,
    document,
    setInterval,
    setTimeout,
    window,
  };

  vm.createContext(context);
  const source = fs.readFileSync(path.join(__dirname, 'knowledge.js'), 'utf8');
  vm.runInContext(source, context, { filename: 'knowledge.js' });

  return { context, getElement, windowListeners };
}

test('knowledge item modal ignores backdrop clicks but still supports explicit close', () => {
  const { context, getElement, windowListeners } = createKnowledgeScriptContext();
  const modal = getElement('knowledge-item-modal');

  context.showAddKnowledgeItemModal();
  assert.equal(modal.style.display, 'block');

  for (const handler of windowListeners.get('click') || []) {
    handler({ target: modal });
  }

  assert.equal(modal.style.display, 'block');

  context.closeKnowledgeItemModal();
  assert.equal(modal.style.display, 'none');
});

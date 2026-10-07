export interface LocalAttachment {
  id: string;
  serviceId: string;
  name: string;
  type: string;
  size: number;
  dataUrl: string;
  createdAt: string;
}

const DB_NAME = 'inovarapp_attachments';
const STORE = 'files';

function openDb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, 1);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(STORE)) db.createObjectStore(STORE, { keyPath: 'id' });
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

export const AttachmentStore = {
  async save(item: Omit<LocalAttachment, 'id' | 'createdAt'>): Promise<LocalAttachment> {
    const value: LocalAttachment = { ...item, id: `${item.serviceId}_${Date.now()}_${Math.random().toString(36).slice(2)}`, createdAt: new Date().toISOString() };
    const db = await openDb();
    await new Promise<void>((resolve, reject) => {
      const req = db.transaction(STORE, 'readwrite').objectStore(STORE).put(value);
      req.onsuccess = () => resolve(); req.onerror = () => reject(req.error);
    });
    db.close();
    return value;
  },
  async list(serviceId: string): Promise<LocalAttachment[]> {
    const db = await openDb();
    const all = await new Promise<LocalAttachment[]>((resolve, reject) => {
      const req = db.transaction(STORE, 'readonly').objectStore(STORE).getAll();
      req.onsuccess = () => resolve((req.result || []).filter((x: LocalAttachment) => x.serviceId === serviceId));
      req.onerror = () => reject(req.error);
    });
    db.close();
    return all;
  },
  async remove(id: string): Promise<void> {
    const db = await openDb();
    await new Promise<void>((resolve, reject) => {
      const req = db.transaction(STORE, 'readwrite').objectStore(STORE).delete(id);
      req.onsuccess = () => resolve(); req.onerror = () => reject(req.error);
    });
    db.close();
  }
};

export function fileToDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

export function attachmentUrl(item: LocalAttachment): string { return item.dataUrl; }

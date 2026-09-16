import { store } from './adapter/supabase/store';
import { memoryStore } from './services/service.memory';

export const client = [store, memoryStore];

import { createClient } from '@supabase/supabase-js';

export const consumer = createClient('https://example.invalid', 'test-key');

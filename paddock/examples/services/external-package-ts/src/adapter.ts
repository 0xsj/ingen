import { createClient } from '@supabase/supabase-js';

export const adapter = createClient('https://example.invalid', 'test-key');

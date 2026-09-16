import { secret } from '../server/secret';
import { Widget } from '../components/widget';
import { browser } from '$app/environment';
import { privateValue } from '$env/static/private';
import { onMount } from 'svelte';
import { error } from '@sveltejs/kit';

export const invalidConfig = { secret, Widget, browser, privateValue, onMount, error };

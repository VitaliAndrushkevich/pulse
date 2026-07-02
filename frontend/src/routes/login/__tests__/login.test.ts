/**
 * Unit tests for the Login page.
 * Validates: Requirements 6.1, 6.4, 6.6, 6.8
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import LoginPage from '../+page.svelte';
import { goto } from '$app/navigation';
import { setToken } from '$lib/stores/auth.svelte';
import { ApiRequestError, NetworkError } from '$lib/api';

// Mock i18n to avoid $effect outside component context
vi.mock('$lib/i18n', () => ({
  t: (key: string, params?: Record<string, string | number>) => {
    const translations: Record<string, string> = {
      'login.title': 'Sign in to Pulse',
      'login.subtitle': 'Enter your credentials to continue',
      'login.email': 'Email',
      'login.emailPlaceholder': 'you@example.com',
      'login.password': 'Password',
      'login.passwordPlaceholder': '••••••••',
      'login.submit': 'Sign in',
      'login.submitting': 'Signing in…',
      'login.errors.invalidCredentials': 'Invalid email or password',
      'login.errors.networkError': 'Service unavailable. Please try again later.',
      'login.errors.unexpected': 'Service unavailable. Please try again later.',
    };
    let result = translations[key] ?? key;
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        result = result.replace(`{${k}}`, String(v));
      }
    }
    return result;
  },
}));

// Mock auth store
vi.mock('$lib/stores/auth.svelte', () => ({
  setToken: vi.fn(),
  getToken: vi.fn(() => null),
  clearToken: vi.fn(),
  isAuthenticated: vi.fn(() => false),
}));

// Mock toast store
vi.mock('$lib/stores/toast.svelte', () => ({
  toastStore: {
    addToast: vi.fn(),
    dismissToast: vi.fn(),
  },
}));

// Mock API
const mockLogin = vi.fn();
vi.mock('$lib/api', async () => {
  const actual = await vi.importActual('$lib/api') as Record<string, unknown>;
  return {
    ...actual,
    login: (...args: unknown[]) => mockLogin(...args),
  };
});

describe('Login Page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders email and password inputs with submit button', () => {
    render(LoginPage);

    expect(screen.getByLabelText('Email')).toBeTruthy();
    expect(screen.getByLabelText('Password')).toBeTruthy();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeTruthy();
  });

  it('disables submit button when form is invalid (empty fields)', () => {
    render(LoginPage);

    const button = screen.getByRole('button', { name: /sign in/i });
    expect(button).toHaveProperty('disabled', true);
  });

  it('disables submit button when email is invalid', async () => {
    render(LoginPage);

    const emailInput = screen.getByLabelText('Email') as HTMLInputElement;
    const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
    const button = screen.getByRole('button', { name: /sign in/i });

    // Set values directly and dispatch input events for Svelte binding
    emailInput.value = 'invalid-email';
    await fireEvent.input(emailInput);
    passwordInput.value = 'password123';
    await fireEvent.input(passwordInput);

    expect(button).toHaveProperty('disabled', true);
  });

  it('disables submit button when password is empty', async () => {
    render(LoginPage);

    const emailInput = screen.getByLabelText('Email') as HTMLInputElement;
    const button = screen.getByRole('button', { name: /sign in/i });

    emailInput.value = 'test@example.com';
    await fireEvent.input(emailInput);

    expect(button).toHaveProperty('disabled', true);
  });

  it('enables submit button when both fields are valid', async () => {
    render(LoginPage);

    const emailInput = screen.getByLabelText('Email') as HTMLInputElement;
    const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
    const button = screen.getByRole('button', { name: /sign in/i });

    emailInput.value = 'test@example.com';
    await fireEvent.input(emailInput);
    passwordInput.value = 'password123';
    await fireEvent.input(passwordInput);

    expect(button).toHaveProperty('disabled', false);
  });

  it('calls login API and redirects on success', async () => {
    mockLogin.mockResolvedValue({ token: 'jwt-token-abc' });

    render(LoginPage);

    const emailInput = screen.getByLabelText('Email') as HTMLInputElement;
    const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
    const button = screen.getByRole('button', { name: /sign in/i });

    emailInput.value = 'user@example.com';
    await fireEvent.input(emailInput);
    passwordInput.value = 'secret';
    await fireEvent.input(passwordInput);
    await fireEvent.click(button);

    await waitFor(() => {
      expect(mockLogin).toHaveBeenCalledWith({ email: 'user@example.com', password: 'secret' });
      expect(setToken).toHaveBeenCalledWith('jwt-token-abc');
      expect(goto).toHaveBeenCalledWith('/');
    });
  });

  it('shows "Invalid email or password" on 401 error', async () => {
    mockLogin.mockRejectedValue(
      new ApiRequestError(401, { code: 'UNAUTHORIZED', message: 'Invalid credentials' }, null)
    );

    render(LoginPage);

    const emailInput = screen.getByLabelText('Email') as HTMLInputElement;
    const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
    const button = screen.getByRole('button', { name: /sign in/i });

    emailInput.value = 'user@example.com';
    await fireEvent.input(emailInput);
    passwordInput.value = 'wrong';
    await fireEvent.input(passwordInput);
    await fireEvent.click(button);

    await waitFor(() => {
      const alert = screen.getByRole('alert');
      expect(alert.textContent).toContain('Invalid email or password');
    });
  });

  it('shows "Service unavailable" on network error', async () => {
    mockLogin.mockRejectedValue(new NetworkError('Connection failed'));

    render(LoginPage);

    const emailInput = screen.getByLabelText('Email') as HTMLInputElement;
    const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
    const button = screen.getByRole('button', { name: /sign in/i });

    emailInput.value = 'user@example.com';
    await fireEvent.input(emailInput);
    passwordInput.value = 'pass';
    await fireEvent.input(passwordInput);
    await fireEvent.click(button);

    await waitFor(() => {
      const alert = screen.getByRole('alert');
      expect(alert.textContent).toContain('Service unavailable. Please try again later.');
    });
  });

  it('keeps field values intact on error', async () => {
    mockLogin.mockRejectedValue(new NetworkError('Network error'));

    render(LoginPage);

    const emailInput = screen.getByLabelText('Email') as HTMLInputElement;
    const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
    const button = screen.getByRole('button', { name: /sign in/i });

    emailInput.value = 'user@example.com';
    await fireEvent.input(emailInput);
    passwordInput.value = 'mypass';
    await fireEvent.input(passwordInput);
    await fireEvent.click(button);

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeTruthy();
    });

    expect(emailInput.value).toBe('user@example.com');
    expect(passwordInput.value).toBe('mypass');
  });

  it('does not reveal which field was wrong on 401', async () => {
    mockLogin.mockRejectedValue(
      new ApiRequestError(401, { code: 'UNAUTHORIZED', message: 'Invalid credentials' }, null)
    );

    render(LoginPage);

    const emailInput = screen.getByLabelText('Email') as HTMLInputElement;
    const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
    const button = screen.getByRole('button', { name: /sign in/i });

    emailInput.value = 'user@example.com';
    await fireEvent.input(emailInput);
    passwordInput.value = 'bad';
    await fireEvent.input(passwordInput);
    await fireEvent.click(button);

    await waitFor(() => {
      const alert = screen.getByRole('alert');
      // Generic message: doesn't reveal which field was wrong
      expect(alert.textContent).toBe('Invalid email or password');
    });
  });
});

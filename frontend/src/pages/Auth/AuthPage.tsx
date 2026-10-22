import React, { useState } from 'react';
import { useAuthStore } from '../../store/useAuthStore';
import {
  Shield,
  Lock,
  Mail,
  User,
  KeyRound,
  ArrowRight,
  Cpu,
  AlertTriangle,
  Loader2,
  CheckCircle2,
} from 'lucide-react';

export const AuthPage: React.FC = () => {
  const { login, signup, isLoading, error, clearError } = useAuthStore();
  const [isSignUp, setIsSignUp] = useState(false);

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [name, setName] = useState('');
  const [role] = useState('SecOps Lead');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    clearError();

    if (isSignUp) {
      if (!name.trim() || !email.trim() || !password.trim()) return;
      await signup(name, email, password, role);
    } else {
      if (!email.trim() || !password.trim()) return;
      await login(email, password);
    }
  };

  return (
    <div className="min-h-screen bg-[#0B0B0C] text-gray-100 flex flex-col justify-center items-center px-4 py-8 relative overflow-hidden font-sans">
      {/* Background Ambient Glow */}
      <div className="absolute top-1/4 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[300px] bg-[#840032]/15 blur-[120px] rounded-full pointer-events-none" />
      <div className="absolute bottom-10 right-10 w-[350px] h-[250px] bg-[#AD2831]/10 blur-[100px] rounded-full pointer-events-none" />

      {/* Brand Header */}
      <div className="mb-6 text-center z-10">
        <div className="inline-flex items-center justify-center p-3 bg-[#151517] border border-[#840032]/50 rounded-2xl shadow-lg mb-3 shadow-[#840032]/10 ring-1 ring-[#840032]/30">
          <Shield className="w-8 h-8 text-[#f07b9e]" />
        </div>
        <div className="flex items-center justify-center space-x-2">
          <h1 className="text-2xl font-bold tracking-tight text-white uppercase">
            AEGIS CONTROL PLANE
          </h1>
          <span className="text-[10px] bg-[#840032]/30 text-[#e2588a] border border-[#840032]/60 px-2 py-0.5 rounded font-mono font-semibold tracking-wide uppercase">
            SECURED
          </span>
        </div>
        <p className="text-xs text-gray-400 mt-1 max-w-md">
          Self-Healing Distributed Systems Platform. Authorized Operator Authentication Required.
        </p>
      </div>

      {/* Main Glassmorphic Auth Card */}
      <div className="w-full max-w-md bg-[#151517]/95 border border-[#26262B] backdrop-blur-xl rounded-xl p-6 sm:p-8 shadow-2xl z-10 transition-all">
        {/* Sign In vs Sign Up Tabs */}
        <div className="flex p-1 bg-[#0B0B0C] border border-[#26262B] rounded-lg mb-5">
          <button
            type="button"
            onClick={() => {
              setIsSignUp(false);
              clearError();
            }}
            className={`flex-1 py-1.5 text-xs font-semibold rounded-md transition-all ${
              !isSignUp
                ? 'bg-[#840032] text-white shadow-sm'
                : 'text-gray-400 hover:text-white'
            }`}
          >
            Sign In
          </button>
          <button
            type="button"
            onClick={() => {
              setIsSignUp(true);
              clearError();
            }}
            className={`flex-1 py-1.5 text-xs font-semibold rounded-md transition-all ${
              isSignUp
                ? 'bg-[#840032] text-white shadow-sm'
                : 'text-gray-400 hover:text-white'
            }`}
          >
            Create Operator Account
          </button>
        </div>

        {/* Error Banner */}
        {error && (
          <div className="mb-4 p-3 bg-[#AD2831]/15 border border-[#AD2831]/50 rounded-lg flex items-center space-x-2 text-xs text-[#f87171]">
            <AlertTriangle className="w-4 h-4 shrink-0 text-[#AD2831]" />
            <span>{error}</span>
          </div>
        )}

        {/* Auth Form */}
        <form onSubmit={handleSubmit} className="space-y-4">
          {isSignUp && (
            <div>
              <label className="block text-[11px] font-semibold text-gray-300 uppercase tracking-wider mb-1.5">
                Operator Full Name
              </label>
              <div className="relative">
                <User className="w-4 h-4 text-gray-500 absolute left-3 top-1/2 -translate-y-1/2" />
                <input
                  type="text"
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. Alex Vance"
                  className="w-full bg-[#0B0B0C] border border-[#26262B] focus:border-[#840032] focus:ring-1 focus:ring-[#840032] rounded-lg pl-9 pr-3 py-2 text-xs text-white placeholder-gray-600 outline-none transition-all"
                />
              </div>
            </div>
          )}

          <div>
            <label className="block text-[11px] font-semibold text-gray-300 uppercase tracking-wider mb-1.5">
              Operator Work Email
            </label>
            <div className="relative">
              <Mail className="w-4 h-4 text-gray-500 absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="operator@aegis.io"
                className="w-full bg-[#0B0B0C] border border-[#26262B] focus:border-[#840032] focus:ring-1 focus:ring-[#840032] rounded-lg pl-9 pr-3 py-2 text-xs text-white placeholder-gray-600 outline-none transition-all"
              />
            </div>
          </div>

          <div>
            <label className="block text-[11px] font-semibold text-gray-300 uppercase tracking-wider mb-1.5">
              Passphrase / Password
            </label>
            <div className="relative">
              <KeyRound className="w-4 h-4 text-gray-500 absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••••••"
                className="w-full bg-[#0B0B0C] border border-[#26262B] focus:border-[#840032] focus:ring-1 focus:ring-[#840032] rounded-lg pl-9 pr-3 py-2 text-xs text-white placeholder-gray-600 outline-none transition-all"
              />
            </div>
          </div>

          {isSignUp && (
            <div className="p-3 bg-[#141416] border border-[#26262B] rounded-lg text-left">
              <div className="flex items-center space-x-2 text-[11px] font-semibold text-gray-300 uppercase tracking-wider mb-1">
                <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
                <span>Server-Enforced RBAC</span>
              </div>
              <p className="text-[11px] text-gray-400 leading-relaxed">
                Initial account is provisioned with <strong className="text-white">Admin</strong> privileges. Subsequent operator accounts default to <strong className="text-white">Viewer</strong> role until elevated by an Administrator.
              </p>
            </div>
          )}

          <button
            type="submit"
            disabled={isLoading}
            className="w-full py-2.5 px-4 bg-[#840032] hover:bg-[#9b053d] text-white rounded-lg text-xs font-semibold uppercase tracking-wider transition-all flex items-center justify-center space-x-2 shadow-lg shadow-[#840032]/25 border border-[#840032]"
          >
            {isLoading ? (
              <>
                <Loader2 className="w-4 h-4 animate-spin text-white" />
                <span>AUTHENTICATING OPERATOR...</span>
              </>
            ) : (
              <>
                <span>{isSignUp ? 'CREATE ACCOUNT & ENTER' : 'AUTHENTICATE & ENTER'}</span>
                <ArrowRight className="w-3.5 h-3.5" />
              </>
            )}
          </button>
        </form>

        {/* Security Encryption Notice */}
        <div className="mt-5 pt-4 border-t border-[#26262B] text-center">
          <p className="text-[11px] text-gray-400">
            {isSignUp
              ? 'Account credentials are cryptographically salted & HMAC-encrypted. No passwords stored in plaintext.'
              : 'Sign in with your registered operator credentials. All actions are cryptographically signed.'}
          </p>
        </div>
      </div>

      {/* Security Footer Badges */}
      <div className="mt-8 flex flex-wrap items-center justify-center gap-6 text-[11px] text-gray-400 z-10 font-mono">
        <div className="flex items-center space-x-1.5">
          <Lock className="w-3.5 h-3.5 text-amber-300" />
          <span>Authority Boundary Gate Protected</span>
        </div>
        <div className="flex items-center space-x-1.5">
          <Cpu className="w-3.5 h-3.5 text-[#e2588a]" />
          <span>TGNN Neural Diagnostics Active</span>
        </div>
        <div className="flex items-center space-x-1.5">
          <CheckCircle2 className="w-3.5 h-3.5 text-[#52b767]" />
          <span>Autonomous Remediation Guardrails</span>
        </div>
      </div>
    </div>
  );
};

"use client";

import { useState, useEffect } from 'react';
import { useMetrics } from '@/hooks/useMetrics';
import { Activity, Shield, ShieldAlert, Zap, ServerCog, CheckCircle, XCircle, KeyRound, Copy, Lock, User, LogOut, BrainCircuit, RefreshCw } from 'lucide-react';
import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts';
import { supabase } from '@/lib/supabase';

export default function Dashboard() {
  const [session, setSession] = useState<any>(null);
  const [loading, setLoading] = useState(true);

  // Auth Form State
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [isLoginMode, setIsLoginMode] = useState(true);
  const [authError, setAuthError] = useState('');

  // Dashboard State
  const [activeTab, setActiveTab] = useState<'DASHBOARD' | 'VAULT'>('DASHBOARD');
  const [cacheEnabled, setCacheEnabled] = useState(true);
  const { metrics, fingerprints, mcpLogs } = useMetrics();

  const currentLatency = metrics.length ? metrics[metrics.length - 1].latency : 0;
  const currentSavings = metrics.length ? metrics[metrics.length - 1].savings : 0;

  // Key Vault State
  const [realKey, setRealKey] = useState('');
  const [generatedKey, setGeneratedKey] = useState<{ virtual_key: string, app_secret: string } | null>(null);
  const [isGenerating, setIsGenerating] = useState(false);
  const [savedKeys, setSavedKeys] = useState<any[]>([]);
  const [rotatingKey, setRotatingKey] = useState<string | null>(null);
  const [newRealKey, setNewRealKey] = useState('');

  useEffect(() => {
    supabase.auth.getSession().then(({ data: { session } }) => {
      setSession(session);
      setLoading(false);
    });

    const { data: { subscription } } = supabase.auth.onAuthStateChange((_event, session) => {
      setSession(session);
    });

    return () => subscription.unsubscribe();
  }, []);

  useEffect(() => {
    if (session?.user?.id && activeTab === 'VAULT') {
      supabase.from('bifrost_keys').select('virtual_key, app_secret, created_at').eq('company_id', session.user.id)
        .then(({ data }) => {
          if (data) setSavedKeys(data);
        });
    }
  }, [session, activeTab]);

  const handleAuth = async (e: React.FormEvent) => {
    e.preventDefault();
    setAuthError('');
    try {
      if (isLoginMode) {
        const { error } = await supabase.auth.signInWithPassword({ email, password });
        if (error) throw error;
      } else {
        const { error } = await supabase.auth.signUp({ email, password });
        if (error) throw error;
        alert('Check your email for the confirmation link!');
      }
    } catch (error: any) {
      setAuthError(error.message);
    }
  };

  const handleGoogleLogin = async () => {
    setAuthError('');
    try {
      const { error } = await supabase.auth.signInWithOAuth({
        provider: 'google',
        options: {
          redirectTo: `${window.location.origin}`
        }
      });
      if (error) throw error;
    } catch (error: any) {
      setAuthError(error.message);
    }
  };

  const proxyUrl = process.env.NEXT_PUBLIC_PROXY_URL || 'http://localhost:8080';

  const generateVirtualKey = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!session?.user?.id) return;
    
    setIsGenerating(true);
    try {
      const res = await fetch(`${proxyUrl}/api/keys/generate`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ 
          real_key: realKey,
          company_id: session.user.id // B2B Multi-Tenant Isolation
        })
      });
      const data = await res.json();
      setGeneratedKey(data);
      setRealKey('');
    } catch (err) {
      console.error(err);
    }
    setIsGenerating(false);
    // Refresh saved keys list
    if (session?.user?.id) {
      supabase.from('bifrost_keys').select('virtual_key, app_secret, created_at').eq('company_id', session.user.id)
        .then(({ data }) => { if (data) setSavedKeys(data); });
    }
  };

  const rotateKey = async (virtualKey: string) => {
    if (!newRealKey) return;
    try {
      await fetch(`${proxyUrl}/api/keys/rotate`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ virtual_key: virtualKey, new_real_key: newRealKey })
      });
      setRotatingKey(null);
      setNewRealKey('');
    } catch (err) {
      console.error(err);
    }
  };

  const toggleCache = async () => {
    if (!session?.user?.id) return;
    const newState = !cacheEnabled;
    setCacheEnabled(newState);
    
    try {
      await fetch(`${proxyUrl}/api/settings/cache`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ 
          company_id: session.user.id,
          cache_enabled: newState 
        })
      });
    } catch (err) {
      console.error("Failed to update cache settings", err);
      setCacheEnabled(!newState); // Revert on failure
    }
  };

  if (loading) {
    return <div className="min-h-screen bg-lumivelle-bg flex items-center justify-center text-lumivelle-accent font-mono tracking-widest">INITIALIZING...</div>;
  }

  // --- LOGIN SCREEN ---
  if (!session) {
    return (
      <main className="min-h-screen bg-lumivelle-bg text-lumivelle-text flex items-center justify-center font-mono p-4">
        <div className="w-full max-w-md border border-lumivelle-border bg-lumivelle-bg p-10 rounded shadow-[0_0_30px_rgba(212,175,55,0.05)]">
          <div className="flex flex-col items-center mb-8">
            <ServerCog className="w-12 h-12 text-lumivelle-accent mb-4" />
            <h1 className="text-2xl font-bold tracking-widest text-lumivelle-accent text-center">BIFRÖST B2B</h1>
            <p className="text-gray-500 mt-2 text-xs uppercase tracking-widest">Sovereign Tenant Access</p>
          </div>

          <form onSubmit={handleAuth} className="space-y-6">
            <div>
              <label className="block text-xs text-gray-400 mb-2 uppercase tracking-widest flex items-center gap-2"><User className="w-3 h-3"/> Corporate Email</label>
              <input 
                type="email" 
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                className="w-full bg-lumivelle-muted/10 border border-lumivelle-border text-lumivelle-text p-3 rounded focus:outline-none focus:border-lumivelle-accent transition-colors"
              />
            </div>
            <div>
              <label className="block text-xs text-gray-400 mb-2 uppercase tracking-widest flex items-center gap-2"><Lock className="w-3 h-3"/> Password</label>
              <input 
                type="password" 
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                className="w-full bg-lumivelle-muted/10 border border-lumivelle-border text-lumivelle-text p-3 rounded focus:outline-none focus:border-lumivelle-accent transition-colors"
              />
            </div>
            
            {authError && <p className="text-red-500 text-xs">{authError}</p>}

            <button type="submit" className="w-full bg-lumivelle-accent text-lumivelle-bg font-bold uppercase tracking-widest p-3 rounded hover:opacity-90 transition-opacity">
              {isLoginMode ? 'Authenticate' : 'Initialize Tenant'}
            </button>
          </form>

          <div className="mt-6 border-t border-lumivelle-border pt-6">
            <button 
              onClick={handleGoogleLogin} 
              className="w-full bg-white text-black font-bold uppercase tracking-widest p-3 rounded hover:bg-gray-200 transition-colors flex items-center justify-center gap-3"
            >
              Sign In with Google
            </button>
          </div>

          <div className="mt-6 text-center">
            <button onClick={() => setIsLoginMode(!isLoginMode)} className="text-gray-500 text-xs uppercase hover:text-lumivelle-accent transition-colors">
              {isLoginMode ? 'Create New Tenant Identity' : 'Return to Authorization'}
            </button>
          </div>
        </div>
      </main>
    );
  }

  // --- MAIN DASHBOARD ---
  return (
    <main className="min-h-screen bg-lumivelle-bg text-lumivelle-text p-8 font-mono flex flex-col">
      <header className="mb-8 border-b border-lumivelle-border pb-6 flex flex-col md:flex-row md:items-end justify-between gap-6">
        <div>
          <h1 className="text-3xl font-bold tracking-widest text-lumivelle-accent flex items-center gap-3">
            <ServerCog className="w-8 h-8" />
            BIFRÖST COMMAND
          </h1>
          <p className="text-gray-500 mt-2 text-sm uppercase tracking-widest flex items-center gap-2">
            Tenant ID: <span className="text-lumivelle-text bg-lumivelle-muted/30 px-2 py-0.5 rounded text-xs">{session.user.id.split('-')[0]}...</span>
          </p>
        </div>
        
        <div className="flex flex-col items-end gap-4">
          <button onClick={() => supabase.auth.signOut()} className="text-gray-500 hover:text-red-400 text-xs flex items-center gap-2 uppercase tracking-widest transition-colors">
            <LogOut className="w-3 h-3" /> Terminate Session
          </button>
          
          <div className="flex items-center gap-8 border-b border-transparent">
            <button 
              onClick={() => setActiveTab('DASHBOARD')}
              className={`pb-2 uppercase tracking-widest text-sm transition-colors ${activeTab === 'DASHBOARD' ? 'text-lumivelle-accent border-b-2 border-lumivelle-accent' : 'text-gray-500 hover:text-gray-300'}`}
            >
              Telemetry
            </button>
            <button 
              onClick={() => setActiveTab('VAULT')}
              className={`pb-2 uppercase tracking-widest text-sm transition-colors flex items-center gap-2 ${activeTab === 'VAULT' ? 'text-lumivelle-accent border-b-2 border-lumivelle-accent' : 'text-gray-500 hover:text-gray-300'}`}
            >
              <KeyRound className="w-4 h-4" /> Key Vault
            </button>
          </div>
        </div>
      </header>

      {/* SEMANTIC BRAIN TOGGLE CONTROL */}
      <div className="mb-8 flex justify-end">
        <button 
          onClick={toggleCache}
          className={`flex items-center gap-3 px-4 py-2 border rounded transition-all ${cacheEnabled ? 'border-green-500/50 bg-green-500/10 text-green-500' : 'border-gray-700 bg-gray-900 text-gray-500'}`}
        >
          <BrainCircuit className="w-4 h-4" />
          <span className="text-xs uppercase tracking-widest font-bold">Semantic Brain: {cacheEnabled ? 'ONLINE' : 'OFFLINE'}</span>
        </button>
      </div>

      {activeTab === 'VAULT' ? (
        <section className="flex-1 flex flex-col items-center justify-center max-w-2xl mx-auto w-full">
           <div className="border border-lumivelle-border bg-lumivelle-muted/10 p-10 rounded w-full relative overflow-hidden">
             <div className="absolute top-0 right-0 p-6 opacity-5">
               <KeyRound className="w-48 h-48 text-lumivelle-accent" />
             </div>
             <h2 className="text-2xl text-lumivelle-accent mb-2 uppercase tracking-widest relative z-10">Generate Virtual Key</h2>
             <p className="text-gray-500 text-sm mb-8 relative z-10">Securely map your real provider API key to a trackable, zero-trust Bifröst credential for this tenant.</p>
             
             <form onSubmit={generateVirtualKey} className="relative z-10">
               <div className="mb-6">
                 <label className="block text-xs text-gray-400 mb-2 uppercase tracking-widest">Real Provider Key</label>
                 <input 
                   type="password" 
                   value={realKey}
                   onChange={(e) => setRealKey(e.target.value)}
                   placeholder="sk-..."
                   required
                   className="w-full bg-lumivelle-bg border border-lumivelle-border text-lumivelle-text p-3 rounded focus:outline-none focus:border-lumivelle-accent transition-colors"
                 />
               </div>
               <button 
                 type="submit" 
                 disabled={isGenerating}
                 className="w-full bg-lumivelle-accent text-lumivelle-bg font-bold uppercase tracking-widest p-3 rounded hover:opacity-90 transition-opacity disabled:opacity-50"
               >
                 {isGenerating ? 'Forging Key...' : 'Forge Virtual Key'}
               </button>
             </form>

             {generatedKey && (
               <div className="mt-8 p-6 border border-green-500/30 bg-green-500/5 rounded relative z-10">
                 <h3 className="text-green-500 text-sm uppercase tracking-widest mb-4 flex items-center gap-2"><CheckCircle className="w-4 h-4" /> Credentials Forged</h3>
                 <div className="space-y-4">
                   <div>
                     <p className="text-xs text-gray-400 mb-1">X-Bifrost-Key</p>
                     <div className="flex items-center justify-between bg-lumivelle-bg p-2 border border-lumivelle-border rounded">
                       <code className="text-sm text-lumivelle-accent">{generatedKey.virtual_key}</code>
                       <button onClick={() => navigator.clipboard.writeText(generatedKey.virtual_key)}><Copy className="w-4 h-4 text-gray-500 hover:text-white" /></button>
                     </div>
                   </div>
                   <div>
                     <p className="text-xs text-gray-400 mb-1">HMAC App Secret (Keep Secure)</p>
                     <div className="flex items-center justify-between bg-lumivelle-bg p-2 border border-lumivelle-border rounded">
                       <code className="text-sm text-lumivelle-accent">{generatedKey.app_secret}</code>
                       <button onClick={() => navigator.clipboard.writeText(generatedKey.app_secret)}><Copy className="w-4 h-4 text-gray-500 hover:text-white" /></button>
                     </div>
                   </div>
                 </div>
               </div>
             )}

             {savedKeys.length > 0 && (
               <div className="mt-8 border-t border-lumivelle-border pt-8 relative z-10">
                 <h3 className="text-gray-400 text-sm uppercase tracking-widest mb-4 flex items-center gap-2"><KeyRound className="w-4 h-4" /> Active Tenant Keys</h3>
                 <div className="space-y-4 max-h-96 overflow-y-auto pr-2 custom-scrollbar">
                   {savedKeys.map((k: any, i: number) => (
                     <div key={i} className="bg-lumivelle-bg border border-lumivelle-border p-4 rounded">
                       <div className="flex justify-between items-center mb-2">
                         <code className="text-xs text-lumivelle-accent">{k.virtual_key}</code>
                         <div className="flex items-center gap-2">
                           <button onClick={() => navigator.clipboard.writeText(k.virtual_key)} title="Copy Key"><Copy className="w-3 h-3 text-gray-500 hover:text-white" /></button>
                           <button 
                             onClick={() => { setRotatingKey(rotatingKey === k.virtual_key ? null : k.virtual_key); setNewRealKey(''); }}
                             title="Rotate Real Key"
                             className={`transition-colors ${rotatingKey === k.virtual_key ? 'text-lumivelle-accent' : 'text-gray-500 hover:text-lumivelle-accent'}`}
                           >
                             <RefreshCw className="w-3 h-3" />
                           </button>
                         </div>
                       </div>
                       <div className="flex justify-between items-center border-t border-lumivelle-border/50 pt-2">
                         <code className="text-xs text-gray-500">{k.app_secret}</code>
                         <button onClick={() => navigator.clipboard.writeText(k.app_secret)}><Copy className="w-3 h-3 text-gray-500 hover:text-white" /></button>
                       </div>

                       {rotatingKey === k.virtual_key && (
                         <div className="mt-3 pt-3 border-t border-lumivelle-accent/30">
                           <label className="block text-xs text-lumivelle-accent mb-2 uppercase tracking-widest">New Real API Key</label>
                           <div className="flex gap-2">
                             <input 
                               type="password"
                               value={newRealKey}
                               onChange={(e) => setNewRealKey(e.target.value)}
                               placeholder="sk-new-..."
                               className="flex-1 bg-lumivelle-muted/10 border border-lumivelle-border text-lumivelle-text p-2 rounded text-xs focus:outline-none focus:border-lumivelle-accent transition-colors"
                             />
                             <button 
                               onClick={() => rotateKey(k.virtual_key)}
                               disabled={!newRealKey}
                               className="bg-lumivelle-accent text-lumivelle-bg font-bold uppercase tracking-widest px-4 py-2 rounded text-xs hover:opacity-90 transition-opacity disabled:opacity-30"
                             >
                               Rotate
                             </button>
                           </div>
                           <p className="text-gray-600 text-xs mt-2">Your virtual key stays the same. Only the underlying provider key changes.</p>
                         </div>
                       )}
                     </div>
                   ))}
                 </div>
               </div>
             )}
           </div>
        </section>
      ) : (
        <>
          {/* THE PULSE & SAVINGS */}
          <section className="mb-12 grid grid-cols-1 lg:grid-cols-3 gap-8">
            <div className="col-span-1 lg:col-span-2 border border-lumivelle-border bg-lumivelle-bg p-6 rounded shadow-[0_0_15px_rgba(212,175,55,0.05)] h-64 flex flex-col">
              <h2 className="text-sm text-gray-500 mb-2 uppercase tracking-widest flex items-center gap-2">
                <Activity className="w-4 h-4 text-lumivelle-accent" />
                The Pulse (Global Latency μs)
              </h2>
              <div className="text-3xl text-lumivelle-accent mb-4">{currentLatency} μs</div>
              <div className="flex-1 w-full">
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={metrics}>
                    <defs>
                      <linearGradient id="colorLatency" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor="#D4AF37" stopOpacity={0.3}/>
                        <stop offset="95%" stopColor="#D4AF37" stopOpacity={0}/>
                      </linearGradient>
                    </defs>
                    <XAxis dataKey="timestamp" hide />
                    <YAxis domain={[0, 'auto']} hide />
                    <Tooltip 
                      contentStyle={{ backgroundColor: '#050505', borderColor: '#332B1A', fontFamily: 'monospace' }}
                      itemStyle={{ color: '#D4AF37' }}
                    />
                    <Area type="step" dataKey="latency" stroke="#D4AF37" fillOpacity={1} fill="url(#colorLatency)" isAnimationActive={false} />
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            </div>

            <div className={`border border-lumivelle-border p-6 rounded flex flex-col justify-center relative overflow-hidden h-64 transition-colors ${cacheEnabled ? 'bg-lumivelle-bg' : 'bg-lumivelle-bg/30 grayscale opacity-50'}`}>
              <div className="absolute top-0 right-0 p-4 opacity-10">
                <Zap className="w-32 h-32 text-lumivelle-accent" />
              </div>
              <h2 className="text-sm text-gray-500 mb-2 uppercase tracking-widest relative z-10">Total Savings</h2>
              <div className="text-5xl font-light text-lumivelle-accent tracking-tighter relative z-10">
                ${currentSavings.toFixed(6)}
              </div>
              <div className="mt-4 flex items-center gap-2 text-xs relative z-10">
                {cacheEnabled ? (
                   <span className="text-green-500 flex items-center gap-1 animate-pulse"><Zap className="w-3 h-3"/> Semantic Cache Active</span>
                ) : (
                   <span className="text-gray-500 flex items-center gap-1"><XCircle className="w-3 h-3"/> Caching Disabled</span>
                )}
              </div>
            </div>
          </section>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
            {/* THE SENTRY */}
            <section className="border border-lumivelle-border bg-lumivelle-muted/10 p-6 rounded min-h-64">
              <h2 className="text-sm text-gray-500 mb-6 uppercase tracking-widest flex items-center gap-2">
                <Shield className="w-4 h-4 text-lumivelle-accent" />
                The Sentry (Zero-Trust Feed)
              </h2>
              <div className="space-y-3">
                {fingerprints.length === 0 && <div className="text-gray-600 text-sm italic">Awaiting telemetry...</div>}
                {fingerprints.map(fp => (
                  <div key={fp.id} className="flex items-center justify-between p-3 border-b border-lumivelle-border/50 bg-lumivelle-bg/50 rounded">
                    <div className="flex items-center gap-3">
                      {fp.status === 'VALID' ? <CheckCircle className="w-4 h-4 text-green-500" /> : 
                      fp.status === 'QUARANTINE' ? <ShieldAlert className="w-4 h-4 text-yellow-500" /> : 
                      <XCircle className="w-4 h-4 text-red-500" />}
                      <span className="text-sm">{fp.fingerprint}</span>
                    </div>
                    <div className="flex flex-col items-end">
                      <span className={`text-xs tracking-widest font-bold ${fp.status === 'VALID' ? 'text-green-500' : fp.status === 'QUARANTINE' ? 'text-yellow-500' : 'text-red-500'}`}>
                        {fp.status}
                      </span>
                      <span className="text-[10px] text-gray-600">{fp.time}</span>
                    </div>
                  </div>
                ))}
              </div>
            </section>

            {/* THE NEGOTIATOR */}
            <section className="border border-lumivelle-border bg-lumivelle-muted/10 p-6 rounded min-h-64">
              <h2 className="text-sm text-gray-500 mb-6 uppercase tracking-widest flex items-center gap-2">
                <ServerCog className="w-4 h-4 text-lumivelle-accent" />
                The Negotiator (MCP Log)
              </h2>
              <div className="space-y-3">
                {mcpLogs.length === 0 && <div className="text-gray-600 text-sm italic">Awaiting agent requests...</div>}
                {mcpLogs.map(log => (
                  <div key={log.id} className="p-3 border border-lumivelle-border/50 bg-lumivelle-bg/50 rounded flex flex-col gap-2">
                    <div className="flex items-center justify-between">
                      <span className="text-sm text-lumivelle-accent">Device {log.device}</span>
                      <span className="text-[10px] text-gray-500">{log.time}</span>
                    </div>
                    <p className="text-xs text-gray-300">Action: {log.action}</p>
                    <div className="flex justify-end">
                      <span className={`text-xs px-2 py-1 rounded tracking-widest ${log.status === 'APPROVED' ? 'bg-green-500/10 text-green-500' : 'bg-red-500/10 text-red-500'}`}>
                        {log.status}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            </section>
          </div>
        </>
      )}
    </main>
  );
}

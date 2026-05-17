export default function SettingsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-white">Settings</h1>
        <p className="text-slate-400">Manage your account, billing, and social connections.</p>
      </div>
      <div className="grid gap-4">
        {["Profile", "Billing", "Social Accounts", "Notifications", "API Keys"].map((section) => (
          <div key={section} className="bg-slate-800 border border-slate-700 rounded-xl p-5">
            <h3 className="text-white font-medium">{section}</h3>
          </div>
        ))}
      </div>
    </div>
  );
}

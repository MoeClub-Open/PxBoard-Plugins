<?php

namespace Plugin\PxboardMigrationBridge;

use App\Models\Plugin;
use App\Models\ServerMachine;
use Illuminate\Http\Request;

class Bridge
{
    public const CODE = 'pxboard_migration_bridge';

    public static function enabled(): bool
    {
        return Plugin::query()->where('code', self::CODE)->where('is_enabled', true)->exists();
    }

    public static function authenticate(Request $request): ServerMachine
    {
        $request->validate(['machine_id' => 'required|integer', 'token' => 'required|string']);
        $machine = ServerMachine::query()
            ->where('id', $request->input('machine_id'))
            ->where('token', $request->input('token'))
            ->first();
        abort_unless($machine && $machine->is_active, 403, 'Machine not found or disabled');
        return $machine;
    }

    public static function redact(array $record): array
    {
        $panel = $record['configuration']['config']['panel'] ?? [];
        $record['revision'] = $record['configuration']['revision'] ?? null;
        unset($record['configuration']);
        $record['target'] = array_diff_key($panel, ['key' => true]);
        return $record;
    }
}

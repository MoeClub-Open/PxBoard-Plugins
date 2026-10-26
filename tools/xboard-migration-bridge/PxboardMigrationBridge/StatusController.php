<?php

namespace Plugin\PxboardMigrationBridge;

use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;

class StatusController
{
    public function receive(Request $request): JsonResponse
    {
        abort_unless(Bridge::enabled(), 404);
        $machine = Bridge::authenticate($request);
        $body = $request->validate([
            'revision' => 'required|integer|min:1',
            'status' => 'required|in:applied,failed',
            'message' => 'nullable|string|max:2000',
        ]);
        Store::update((int) $machine->id, function (?array $record) use ($body, $machine): array {
            abort_unless($record && $record['configuration']['revision'] === (int) $body['revision'], 409, 'Configuration revision changed');
            abort_unless($record['status'] === 'pending' || $record['status'] === $body['status'], 409, 'Configuration already handled');
            $key = $record['configuration']['config']['panel']['key'];
            $record['status'] = $body['status'];
            $record['message'] = str_replace([$key, $machine->token], '[redacted]', $body['message'] ?? '');
            $record['updatedAt'] = gmdate('c');
            return $record;
        });
        return response()->json(['data' => true]);
    }
}

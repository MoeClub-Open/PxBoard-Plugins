<?php

namespace Plugin\PxboardMigrationBridge;

use Closure;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Log;
use Throwable;

class MachineResponse
{
    public function handle(Request $request, Closure $next)
    {
        $response = $next($request);
        if (!$request->isMethod('POST') || $request->path() !== 'api/v2/server/machine/nodes'
            || !$response instanceof JsonResponse || !$response->isSuccessful() || !Bridge::enabled()) {
            return $response;
        }
        $machine = Bridge::authenticate($request);
        try {
            $task = Store::read((int) $machine->id);
        } catch (Throwable $error) {
            Log::warning('Cannot read NodeRS migration task', ['machine_id' => $machine->id]);
            return $response;
        }
        if (($task['status'] ?? null) === 'pending') {
            $data = $response->getData(true);
            $data['agent_config']['managed_configuration'] = $task['configuration'];
            $response->setData($data);
            $response->headers->set('Cache-Control', 'no-store');
        }
        return $response;
    }
}

<?php

namespace Plugin\PxboardMigrationBridge;

use App\Services\Plugin\AbstractPlugin;

class Plugin extends AbstractPlugin
{
    public function boot(): void
    {
        app('router')->pushMiddlewareToGroup('api', MachineResponse::class);
    }
}

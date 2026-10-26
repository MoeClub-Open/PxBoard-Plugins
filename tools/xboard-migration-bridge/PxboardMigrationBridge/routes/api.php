<?php

use Illuminate\Support\Facades\Route;
use Plugin\PxboardMigrationBridge\StatusController;

Route::post('/api/v2/server/machine/configuration/status', ['\\' . StatusController::class, 'receive']);

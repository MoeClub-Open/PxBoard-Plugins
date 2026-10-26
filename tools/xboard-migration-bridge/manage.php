<?php

use App\Models\ServerMachine;
use App\Services\Plugin\PluginManager;
use Illuminate\Contracts\Console\Kernel;
use Plugin\PxboardMigrationBridge\Bridge;
use Plugin\PxboardMigrationBridge\Store;

if (PHP_SAPI !== 'cli') {
    http_response_code(404);
    exit;
}

function copyPlugin(string $source, string $target): void
{
    if (!is_dir($target) && !mkdir($target, 0755, true) && !is_dir($target)) {
        throw new RuntimeException('Cannot create plugin directory');
    }
    foreach (scandir($source) as $entry) {
        if ($entry === '.' || $entry === '..') {
            continue;
        }
        if (is_dir($source . '/' . $entry)) {
            copyPlugin($source . '/' . $entry, $target . '/' . $entry);
        } elseif (!copy($source . '/' . $entry, $target . '/' . $entry)) {
            throw new RuntimeException('Cannot copy plugin file');
        }
    }
}

try {
    $root = isset($argv[1]) ? realpath($argv[1]) : false;
    $command = $argv[2] ?? '';
    if (!$root || !is_file($root . '/artisan') || !is_file($root . '/vendor/autoload.php')) {
        throw new RuntimeException('Usage: php manage.php /path/to/xboard install|enable|disable|uninstall|queue|status [machine-id] [target.json]');
    }
    if ($command === 'install') {
        $target = $root . '/plugins/PxboardMigrationBridge';
        if (file_exists($target)) {
            throw new RuntimeException('Plugin directory already exists; use enable for an existing installation');
        }
        copyPlugin(__DIR__ . '/PxboardMigrationBridge', $target);
    }
    require $root . '/vendor/autoload.php';
    $app = require $root . '/bootstrap/app.php';
    $app->make(Kernel::class)->bootstrap();
    $manager = $app->make(PluginManager::class);
    $code = 'pxboard_migration_bridge';
    switch ($command) {
        case 'install':
            $manager->install($code);
            $manager->enable($code);
            echo "Bridge installed and enabled\n";
            break;
        case 'enable':
        case 'disable':
        case 'uninstall':
            $manager->{$command}($code);
            echo "Bridge {$command} complete\n";
            break;
        case 'queue':
            $machineId = filter_var($argv[3] ?? '', FILTER_VALIDATE_INT, ['options' => ['min_range' => 1]]);
            if (!$machineId || !ServerMachine::query()->whereKey($machineId)->where('is_active', true)->exists()) {
                throw new RuntimeException('Source machine not found or disabled');
            }
            if (!Bridge::enabled()) {
                throw new RuntimeException('Bridge plugin is disabled');
            }
            $input = json_decode(file_get_contents($argv[4] ?? ''), true, 512, JSON_THROW_ON_ERROR);
            $panel = $input['panel'] ?? [];
            $url = parse_url($panel['api'] ?? '');
            if (!in_array($panel['type'] ?? '', ['pxboard', 'xboard'], true)
                || !is_array($url) || !in_array($url['scheme'] ?? '', ['http', 'https'], true) || empty($url['host'])
                || isset($url['user']) || isset($url['pass']) || isset($url['query']) || isset($url['fragment'])
                || !is_string($panel['key'] ?? null) || $panel['key'] === ''
                || !is_int($panel['machine_id'] ?? null) || $panel['machine_id'] < 1) {
                throw new RuntimeException('Target JSON requires panel.type, panel.api, panel.key, panel.machine_id');
            }
            $panel = array_intersect_key($panel, array_flip(['type', 'api', 'key', 'machine_id']));
            $panel['api'] = rtrim($panel['api'], '/');
            $record = Store::update($machineId, function (?array $old) use ($machineId, $panel): array {
                if (($old['status'] ?? null) === 'pending') {
                    throw new RuntimeException('A configuration is already pending for this machine');
                }
                return [
                    'machineId' => $machineId,
                    'configuration' => ['revision' => ($old['configuration']['revision'] ?? 0) + 1, 'config' => ['panel' => $panel]],
                    'status' => 'pending', 'message' => '', 'updatedAt' => gmdate('c'),
                ];
            });
            echo json_encode(Bridge::redact($record), JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR) . "\n";
            break;
        case 'status':
            $machineId = filter_var($argv[3] ?? '', FILTER_VALIDATE_INT, ['options' => ['min_range' => 1]]);
            if (!$machineId) {
                throw new RuntimeException('Provide a positive source machine ID');
            }
            $record = Store::read($machineId);
            echo json_encode($record ? Bridge::redact($record) : null, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR) . "\n";
            break;
        default:
            throw new RuntimeException('Unknown command');
    }
} catch (Throwable $error) {
    fwrite(STDERR, $error->getMessage() . "\n");
    exit(1);
}

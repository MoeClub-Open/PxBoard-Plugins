import { existsSync } from "node:fs";
import { isAbsolute } from "node:path";
import { DatabaseSync } from "node:sqlite";
import { parseArgs } from "node:util";

const { values } = parseArgs({
  options: {
    database: { type: "string" },
    apply: { type: "boolean", default: false },
  },
});
if (
  !values.database ||
  !isAbsolute(values.database) ||
  !existsSync(values.database)
)
  throw new Error(
    "请通过 --database 指定现有数据库的绝对路径；停止面板后用 --apply 写入，默认仅检查",
  );
const db = new DatabaseSync(values.database);
type Row = { namespace: string; collection: string; id: string; data: string };
type Event = { type: string; data: Record<string, unknown>; operation: string };
const now = new Date().toISOString();
db.exec("BEGIN IMMEDIATE");
try {
  const recipients = (
    db.prepare("SELECT id,manifest FROM plugins WHERE enabled=1").all() as {
      id: string;
      manifest: string;
    }[]
  ).map((p) => ({ id: p.id, events: JSON.parse(p.manifest).events || [] }));
  const rows = db
    .prepare(
      "SELECT namespace,collection,id,data FROM documents WHERE (namespace='commerce' AND collection='orders' AND (json_type(data,'$.notificationPending') IS NOT NULL OR json_type(data,'$.refundNotificationPending') IS NOT NULL)) OR (namespace='support' AND collection='tickets' AND json_type(data,'$.pendingNotifications') IS NOT NULL)",
    )
    .all() as Row[];
  let queued = 0;
  for (const row of rows) {
    const value = JSON.parse(row.data),
      events: Event[] = [];
    if (row.namespace === "commerce") {
      if (value.notificationPending)
        events.push({
          type: "order.fulfilled",
          operation: `order-fulfilled:${row.id}`,
          data: Object.fromEntries(
            ["userId", "amount", "planName", "kind", "paidAt", "fulfilledAt"]
              .map((k) => [k, value[k]])
              .concat([["orderId", row.id]]),
          ),
        });
      if (value.refundNotificationPending)
        events.push({
          type: "order.refunded",
          operation: `order-refunded:${row.id}`,
          data: {
            orderId: row.id,
            userId: value.userId,
            amount: value.amount,
            refundedAt: value.refundedAt,
          },
        });
      delete value.notificationPending;
      delete value.refundNotificationPending;
    } else {
      for (const data of value.pendingNotifications || [])
        events.push({
          type: "ticket.replied",
          operation: `ticket:${data.messageId}`,
          data,
        });
      delete value.pendingNotifications;
    }
    for (const event of events) {
      const id = `migrate-outbox:${row.namespace}:${event.operation}`;
      db.prepare(
        "INSERT OR IGNORE INTO events(id,type,source,data,created_at) VALUES(?,?,?,?,?)",
      ).run(id, event.type, row.namespace, JSON.stringify(event.data), now);
      for (const recipient of recipients)
        if (recipient.events.includes(event.type))
          db.prepare(
            "INSERT OR IGNORE INTO deliveries(event_id,plugin_id,next_at) VALUES(?,?,?)",
          ).run(id, recipient.id, now);
      db.prepare(
        "INSERT OR IGNORE INTO mutations(namespace,id,created_at) VALUES(?,?,?)",
      ).run(row.namespace, `commit:${event.operation}`, now);
      queued++;
    }
    db.prepare(
      "UPDATE documents SET data=?,updated_at=? WHERE namespace=? AND collection=? AND id=?",
    ).run(JSON.stringify(value), now, row.namespace, row.collection, row.id);
  }
  // Support no longer scans tickets on a timer; its former timer deliveries
  // carry no business action once the pending notifications above are queued.
  db.prepare(
    "UPDATE deliveries SET done=1,error='' WHERE plugin_id='support' AND done=0 AND event_id IN (SELECT id FROM events WHERE type='system.tick' AND source='core')",
  ).run();
  db.exec(values.apply ? "COMMIT" : "ROLLBACK");
  process.stdout.write(
    JSON.stringify({ apply: values.apply, records: rows.length, queued }) +
      "\n",
  );
} catch (error) {
  db.exec("ROLLBACK");
  throw error;
} finally {
  db.close();
}

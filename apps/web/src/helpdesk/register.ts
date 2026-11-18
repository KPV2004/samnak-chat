import { cardRenderers } from "../chat/cards";
import { HelpdeskCardGate } from "./HelpdeskCard";

// Imported once by App.tsx: the chat then shows helpdesk.ticket cards with this renderer.
cardRenderers["helpdesk.ticket"] = HelpdeskCardGate;

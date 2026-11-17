import { cardRenderers } from "../chat/cards";
import { ApprovalCardGate } from "./ApprovalCard";

// Imported once by App.tsx: the chat then shows approval.request cards with this renderer.
cardRenderers["approval.request"] = ApprovalCardGate;
